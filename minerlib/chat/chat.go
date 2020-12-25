package chat

import (
	"github.com/cryptonote-social/csminer/crylog"
	"github.com/cryptonote-social/csminer/stratum/client"

	"sync"
)

var (
	mutex sync.Mutex

	chatQueue       []string
	chatToSendIndex int

	receivedQueue     []*client.ChatResult
	chatReceivedIndex int

	nextToken int
)

func SendChat(chat string) {
	mutex.Lock()
	defer mutex.Unlock()
	chatQueue = append(chatQueue, chat)
	crylog.Info("Chat queued for sending:", chat)
}

// GetChatToSend returns the next queued chat message that needs to be delivered.  The function
// will return the same result until ChatSent is called. It will return (nil, -1) if there are no
// chats to send at this time.
func GetChatToSend() (chat string, id int) {
	mutex.Lock()
	defer mutex.Unlock()
	if chatToSendIndex >= len(chatQueue) {
		return "", -1
	}
	return chatQueue[chatToSendIndex], chatToSendIndex
}

func HasChatsToSend() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return chatToSendIndex < len(chatQueue)
}

func ChatSent(id int) {
	mutex.Lock()
	defer mutex.Unlock()
	if id == chatToSendIndex {
		crylog.Info("Chat message delivered:", chatQueue[id])
		chatToSendIndex++
	}
}

func ChatsReceived(chats []client.ChatResult, chatToken int, fetchedToken int) {
	if len(chats) == 0 {
		return
	}
	crylog.Info("Chats received:", chats)
	mutex.Lock()
	defer mutex.Unlock()
	if nextToken != fetchedToken {
		crylog.Warn("Skipping dupe chats:", chats)
		return // these chats are already handled
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

func NextToken() int {
	mutex.Lock()
	defer mutex.Unlock()
	return nextToken
}
