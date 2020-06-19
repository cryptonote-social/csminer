// Copyright 2020 cryptonote.social. All rights reserved. Use of this source code is governed by
// the license found in the LICENSE file.
package blockchain

// blockchain/json-rpc.go supports making json rpc calls to the blockchain daemon.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cryptonote-social/csminer/crylog"
	"io/ioutil"
	"net/http"
	"strings"
)

var (
	NOT_SYNCED_ERR error = errors.New("daemon not synced")
)

type JSONRequest struct {
	Jsonrpc  string      `json:"jsonrpc"`
	Method   string      `json:"method"`
	Password string      `json:"password"`
	Params   interface{} `json:"params"`
	ID       uint64      `json:"id"`
}

type JSONResponse struct {
	ID     uint64
	Result *json.RawMessage
	Error  JSONError
}

type JSONError struct {
	Code    int
	Message string
}

func DoJSONRPC(client *http.Client, urlString string, jReq *JSONRequest, jResp *JSONResponse, result interface{}) error {
	jReq.Jsonrpc = "2.0"
	data, err := json.Marshal(jReq)
	if err != nil {
		crylog.Error("couldn't marshal json:", err)
		return err
	}
	var resp *http.Response
	resp, err = client.Post(urlString, "application/json", bytes.NewReader(data))
	if err != nil {
		crylog.Error("post failed:", err)
		return err
	}

	jResp.Error.Code = 0
	jResp.Result = nil
	body, err := ioutil.ReadAll(resp.Body)
	//err = json.NewDecoder(resp.Body).Decode(jResp)
	err2 := resp.Body.Close()
	if err2 != nil {
		crylog.Error("failed to close body:", err2)
	}
	err = json.Unmarshal(body, jResp)
	if err != nil {
		// Sometimes TRTL daemon returns responses of the following format:
		// {"status":"Failed","error":"Daemon must be synced to process this RPC method call, please retry when synced"}
		r := &struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}{}
		err2 = json.Unmarshal(body, r)
		if err2 != nil {
			crylog.Error("failed to decode outer json response:", err, "body:", string(body))
			return err
		}
		if strings.HasPrefix(r.Error, "Daemon must be synced") {
			crylog.Warn("Ignoring daemon not synced error")
			return NOT_SYNCED_ERR
		}
		crylog.Error("unknown daemon error, body:", string(body))
		return err
	}
	if jResp.Error.Code != 0 {
		crylog.Error("non-zero response error code from server:", jResp.Error.Code)
		return fmt.Errorf("json response error %v with message: %v",
			jResp.Error.Code, jResp.Error.Message)
	}
	if jResp.Result == nil {
		return fmt.Errorf("JSONResponse.Result was unexpectedly nil")
	}
	err = json.Unmarshal(*jResp.Result, result)
	if err != nil {
		crylog.Error("failed to unmarshal json result:", err, "::", string(*jResp.Result))
		return err
	}
	return nil
}
