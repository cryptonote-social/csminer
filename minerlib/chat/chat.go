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
	crylog.Info("Chat queued for sending:", chat)
	return int64(len(chatQueue)-1) ^ randID
}

// GetChatToSend returns the next queued chat message that needs to be delivered.  The function
// will return the same result until ChatSent is called. It will return ("", -1) if there are no
// chats to send at this time.
func GetChatToSend() (chat string, id int64) {
	mutex.Lock()
	defer mutex.Unlock()
	if chatToSendIndex >= len(chatQueue) {
		return "", -1
	}
	crylog.Info("ID:", int64(chatToSendIndex)^randID, randID)
	return chatQueue[chatToSendIndex], int64(chatToSendIndex) ^ randID
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

func ChatsReceived(chats []client.ChatResult, chatToken int64, fetchedToken int64) {
	if len(chats) != 0 {
		crylog.Info("Chats received:", chats)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if nextToken != fetchedToken {
		// Another chat request must have succeeded before this one.
		crylog.Warn("chats updated since this fetch, discarding:", chats)
		return
	}
	for i := range chats {
		receivedQueue = append(receivedQueue, &chats[i])
	}
	nextToken = chatToken
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
