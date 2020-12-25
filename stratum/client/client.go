// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.

// package client implements a basic stratum client that listens to jobs and
// can submit shares
package client

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cryptonote-social/csminer/crylog"
	"io"
	"net"
	"sync"
	"time"
)

const (
	SUBMIT_WORK_JSON_ID = 999
	CONNECT_JSON_ID     = 666
	GET_CHATS_JSON_ID   = 9999

	MAX_REQUEST_SIZE = 50000 // Max # of bytes we will read per request

	NO_WALLET_SPECIFIED_WARNING_CODE = 2
)

type Job struct {
	Blob   string `json:"blob"`
	JobID  string `json:"job_id"`
	Target string `json:"target"`
	Algo   string `json:"algo"`
	// For self-select mode:
	PoolWallet string `json:"pool_wallet"`
	ExtraNonce string `json:"extra_nonce"`

	ChatToken int `json:"chat_token"` // custom field
}

type RXJob struct {
	Job
	Height   int    `json:"height"`
	SeedHash string `json:"seed_hash"`
}

type ForknoteJob struct {
	Job
	MajorVersion int `json:"blockMajorVersion"`
	MinorVersion int `json:"blockMinerVersion"`
}

type MultiClientJob struct {
	RXJob
	NetworkDifficulty int64  `json:"net_diff"`
	Reward            int64  `json:"reward"`
	ConnNonce         uint32 `json:"nonce"`
}

type SubmitWorkResult struct {
	Status string `json:"status"`

	Progress       float64 // progress of this user
	LifetimeHashes int64   // hashes from this user over its lifetime
	Paid           float64 // Total crypto paid to this user over its lifetime.
	Owed           float64 // Crypto owed to this user but not paid out yet.

	Donate float64 // Fraction of earnings donated to the pool by this user.

	PPROPHashrate     int64   // hashrate of the pprop collective
	PPROPProgress     float64 // raw progress of the pprop collective
	NextBlockReward   float64 // reward of the next banked block
	NetworkDifficulty int64   // difficulty, possibly smoothed over the last several blocks

	// TODO: These pool config values rarely change, so we should fetch these in some other way
	// instead of returning them from each SubmitWork call, or perhaps only return them when
	// they change.
	PoolMargin float64
	PoolFee    float64
}

type ChatResult struct {
	Username  string // user sending the chat
	Message   string // the chat message
	Timestamp int64  // unix time in seconds when the chat message was sent
}

type GetChatsResult struct {
	Chats     []ChatResult
	NextToken int
}

type Response struct {
	ID      uint64 `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`

	Job    *MultiClientJob  `json:"params"` // used to send jobs over the connection
	Result *json.RawMessage `json:"result"` // used to return SubmitWork or GetChats results

	Error interface{} `json:"error"`

	ChatToken int `json:"chat_token"` // custom field
}

type loginResponse struct {
	ID      uint64 `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Result  *struct {
		ID  string          `json:"id"`
		Job *MultiClientJob `job:"job"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	// our own custom field for reporting login warnings without forcing disconnect from error:
	Warning *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"warning"`

	ChatToken int `json:"chat_token"` // custom field
}

type Client struct {
	address         string
	conn            net.Conn
	responseChannel chan *Response

	mutex sync.Mutex

	alive bool // true when the stratum client is connected. Set to false upon call to Close(), or when Connect() is called but
	// a new connection is yet to be established.
}

func (cl *Client) String() string {
	// technically this should be locked to access address, but then
	// we risk deadlock if we try to use it when client is already locked.
	return "client:" + cl.address
}

func (cl *Client) IsAlive() bool {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	return cl.alive
}

// Connect to the stratum server port with the given login info. Returns error if connection could
// not be established, or if the stratum server itself returned an error. In the latter case,
// code and message will also be specified. If the stratum server returned just a warning, then
// error will be nil, but code & message will be specified.
func (cl *Client) Connect(
	address string, useTLS bool, agent string,
	uname, pw, rigid string) (err error, code int, message string, jobChan <-chan *MultiClientJob) {
	cl.Close() // just in case caller forgot to call close before trying a new connection
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	cl.address = address

	if !useTLS {
		cl.conn, err = net.DialTimeout("tcp", address, time.Second*30)
	} else {
		cl.conn, err = tls.Dial("tcp", address, nil /*Config*/)
	}
	if err != nil {
		crylog.Error("Dial failed:", err, cl)
		return err, 0, "", nil
	}
	// send login
	loginRequest := &struct {
		ID     uint64      `json:"id"`
		Method string      `json:"method"`
		Params interface{} `json:"params"`
	}{
		ID:     CONNECT_JSON_ID,
		Method: "login",
		Params: &struct {
			Login string `json:"login"`
			Pass  string `json:"pass"`
			RigID string `json:"rigid"`
			Agent string `json:"agent"`
		}{
			Login: uname,
			Pass:  pw,
			RigID: rigid,
			Agent: agent,
		},
	}

	data, err := json.Marshal(loginRequest)
	if err != nil {
		crylog.Error("json marshalling failed:", err, "for client")
		return err, 0, "", nil
	}
	cl.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	data = append(data, '\n')
	if _, err = cl.conn.Write(data); err != nil {
		crylog.Error("writing request failed:", err, "for client")
		return err, 0, "", nil
	}

	// Now read the login response
	response := &loginResponse{}
	cl.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	rdr := bufio.NewReaderSize(cl.conn, MAX_REQUEST_SIZE)
	err = readJSON(response, rdr)
	if err != nil {
		crylog.Error("readJSON failed for client:", err)
		return err, 0, "", nil
	}
	if response.Result == nil {
		crylog.Error("Didn't get job result from login response:", response.Error)
		return errors.New("stratum server error"), response.Error.Code, response.Error.Message, nil
	}

	cl.responseChannel = make(chan *Response)
	cl.alive = true
	jc := make(chan *MultiClientJob)
	response.Result.Job.ChatToken = response.ChatToken
	go dispatchJobs(cl.conn, jc, response.Result.Job, cl.responseChannel)
	if response.Warning != nil {
		return nil, response.Warning.Code, response.Warning.Message, jc
	}
	return nil, 0, "", jc
}

// if error is returned then client will be closed and put in not-alive state
func (cl *Client) SubmitMulticlientWork(username string, rigid string, nonce string, connNonce []byte, jobid string, targetDifficulty int64) (*Response, error) {
	submitRequest := &struct {
		ID     uint64      `json:"id"`
		Method string      `json:"method"`
		Params interface{} `json:"params"`
	}{
		ID:     SUBMIT_WORK_JSON_ID,
		Method: "submit",
		Params: &struct {
			ID     string `json:"id"`
			JobID  string `json:"job_id"`
			Nonce  string `json:"nonce"`
			Result string `json:"result"`
			// Fields below are used by profit-maximizing servicea
			ForUser       string `json:"for_user"`
			ForRig        string `json:"for_rig"`
			ForDifficulty int64  `json:"for_difficulty"`
			ConnNonce     []byte `json:"conn_nonce"`
		}{"696969", jobid, nonce, "", username, rigid, targetDifficulty, connNonce},
	}

	return cl.submitRequest(submitRequest, SUBMIT_WORK_JSON_ID)
}

// if error is returned then client will be closed and put in not-alive state
func (cl *Client) submitRequest(submitRequest interface{}, expectedResponseID uint64) (*Response, error) {
	cl.mutex.Lock()
	if !cl.alive {
		cl.mutex.Unlock()
		return nil, errors.New("client not alive")
	}
	data, err := json.Marshal(submitRequest)
	if err != nil {
		crylog.Error("json marshalling failed:", err, "for client")
		cl.mutex.Unlock()
		return nil, err
	}
	cl.conn.SetWriteDeadline(time.Now().Add(60 * time.Second))
	data = append(data, '\n')
	if _, err = cl.conn.Write(data); err != nil {
		crylog.Error("writing request failed:", err, "for client")
		cl.mutex.Unlock()
		return nil, err
	}
	respChan := cl.responseChannel
	cl.mutex.Unlock()

	// await the response
	response := <-respChan
	if response == nil {
		crylog.Error("got nil response, closing")
		return nil, fmt.Errorf("submit work failure: nil response")
	}
	if response.ID != expectedResponseID {
		crylog.Error("got unexpected response:", response.ID, "wanted:", expectedResponseID, "Closing connection.")
		return nil, fmt.Errorf("submit work failure: unexpected response")
	}
	return response, nil
}

func (cl *Client) GetChats(chatToken int) (*Response, error) {
	chatRequest := &struct {
		ID     uint64      `json:"id"`
		Method string      `json:"method"`
		Params interface{} `json:"params"`
	}{
		ID:     GET_CHATS_JSON_ID,
		Method: "get_chats",
		Params: &struct {
			ChatToken int `json:"chat_token"`
		}{chatToken},
	}

	return cl.submitRequest(chatRequest, GET_CHATS_JSON_ID)
}

// if error is returned then client will be closed and put in not-alive state
func (cl *Client) SubmitWork(nonce string, jobid string, chat string, chatID int) (*Response, error) {
	submitRequest := &struct {
		ID     uint64      `json:"id"`
		Method string      `json:"method"`
		Params interface{} `json:"params"`
	}{
		ID:     SUBMIT_WORK_JSON_ID,
		Method: "submit",
		Params: &struct {
			ID     string `json:"id"`
			JobID  string `json:"job_id"`
			Nonce  string `json:"nonce"`
			Result string `json:"result"`

			Chat   string `json:"chat"`
			ChatID int    `json:"chat_id"`
		}{"696969", jobid, nonce, "", chat, chatID},
	}
	return cl.submitRequest(submitRequest, SUBMIT_WORK_JSON_ID)
}

func (cl *Client) Close() {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	if !cl.alive {
		return
	}
	cl.alive = false
	cl.conn.Close()
}

// dispatchJobs will forward incoming jobs to the JobChannel until error is received or the
// connection is closed. Client will be in not-alive state on return.
func dispatchJobs(conn net.Conn, jobChan chan<- *MultiClientJob, firstJob *MultiClientJob, responseChan chan<- *Response) {
	defer func() {
		close(jobChan)
		close(responseChan)
	}()
	jobChan <- firstJob
	reader := bufio.NewReaderSize(conn, MAX_REQUEST_SIZE)
	for {
		response := &Response{}
		conn.SetReadDeadline(time.Now().Add(3600 * time.Second))
		err := readJSON(response, reader)
		if err != nil {
			crylog.Error("readJSON failed, closing client:", err)
			break
		}
		if response.Method != "job" {
			if response.ID == SUBMIT_WORK_JSON_ID || response.ID == GET_CHATS_JSON_ID {
				responseChan <- response
				continue
			}
			crylog.Warn("Unexpected response from stratum server. Ignoring:", *response)
			continue
		}
		if response.Job == nil {
			crylog.Error("Didn't get job as expected from stratum server, closing client:", *response)
			break
		}
		response.Job.ChatToken = response.ChatToken
		jobChan <- response.Job
	}
}

func readJSON(response interface{}, reader *bufio.Reader) error {
	data, isPrefix, err := reader.ReadLine()
	if isPrefix {
		crylog.Warn("oversize request")
		return errors.New("oversize request")
	} else if err == io.EOF {
		crylog.Info("eof")
		return err
	} else if err != nil {
		crylog.Warn("error reading:", err)
		return err
	}
	err = json.Unmarshal(data, response)
	if err != nil {
		crylog.Warn("failed to unmarshal json stratum login response:", err)
		return err
	}
	return nil
}
