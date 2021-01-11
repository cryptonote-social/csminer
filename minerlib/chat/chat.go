package chat

import (
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/stratum/client"

	"crypto/rand"
	"encoding/binary"
	"math"
	"sync"
)

var (
	mutex sync.Mutex

	chatQueue       []string
	chatToSendIndex int

	receivedQueue     []*client.ChatResult
	chatReceivedIndex int

	nextToken int64

	randID int64
)

const (
	HASHES_PER_CHAT     = 5000
	MAX_CHATS_PER_SHARE = 5
)

func init() {
	err := binary.Read(rand.Reader, binary.LittleEndian, &randID)
	if err != nil {
		crylog.Fatal("Init error for randID:", err)
	}
	// get rid of negative sign just for aesthetics
	randID &= math.MaxInt64
}

// Queue a chat for sending, returning the id token of the chat
func SendChat(chat string) int64 {
	mutex.Lock()
	defer mutex.Unlock()
	chatQueue = append(chatQueue, chat)
	return int64(len(chatQueue)-1) ^ randID
}

// GetChatsToSend returns the next queud chat messages to deliver with a valid mining share.  It
// requires at least HASHES_PER_CHAT hashes to be computed per chat returned, and returns up to
// MAX_CHATS_PER_SHARE. Returns nil if there are no chats queued to send.
func GetChatsToSend(diff int64) []client.ChatToSend {
	mutex.Lock()
	defer mutex.Unlock()
	if chatToSendIndex == len(chatQueue) {
		return nil
	}
	r := []client.ChatToSend{}
	for diff >= HASHES_PER_CHAT && chatToSendIndex < len(chatQueue) && len(r) < MAX_CHATS_PER_SHARE {
		r = append(r, client.ChatToSend{
			ID:      int64(chatToSendIndex) ^ randID,
			Message: chatQueue[chatToSendIndex],
		})
		chatToSendIndex++
		diff -= HASHES_PER_CHAT
	}
	// TODO: verify the total bytes we will be sending is within the server's request size limit
	return r
}

func HasChatsToSend() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return chatToSendIndex < len(chatQueue)
}

func ChatSent(id int64) {
	mutex.Lock()
	defer mutex.Unlock()
	if id == int64(chatToSendIndex)^randID {
		chatToSendIndex++
	}
}

// ChatsReceived should be called by whenever the server returns a GetChatsResult. tokenSent should
// be set to the value of NextToken that was used in the request to the server that produced the
// GetChatsResult response.
func ChatsReceived(cr *client.GetChatsResult, tokenSent int64) {
	if len(cr.Chats) == 0 && cr.NextToken == tokenSent {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()
	if nextToken != tokenSent {
		// Another chat request must have succeeded before this one.
		crylog.Warn("chats updated since this fetch, discarding:", cr.Chats)
		return
	}
	for i := range cr.Chats {
		receivedQueue = append(receivedQueue, &cr.Chats[i])
	}
	nextToken = cr.NextToken
}

func HasChats() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return chatReceivedIndex < len(receivedQueue)
}

func NextChatReceived() *client.ChatResult {
	mutex.Lock()
	defer mutex.Unlock()
	if chatReceivedIndex < len(receivedQueue) {
		chatReceivedIndex++
		return receivedQueue[chatReceivedIndex-1]
	}
	return nil
}

func NextToken() int64 {
	mutex.Lock()
	defer mutex.Unlock()
	return nextToken
}
