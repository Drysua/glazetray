package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/getlantern/systray"
	"github.com/gorilla/websocket"
)

type WmEventType string
type WmEventData map[string]interface{}
type ServerMessage struct {
	MessageType    string      `json:"messageType"`
	ClientMessage  string      `json:"clientMessage"`
	Data           interface{} `json:"data"`
	Error          interface{} `json:"error"`
	Success        bool        `json:"success"`
	SubscriptionId string      `json:"subscriptionId"`
}

var wsConn *websocket.Conn
var mutex sync.Mutex

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	go connectWebSocket()
	loadIcon("default")
	systray.SetTitle("GlazeTray")
	systray.SetTooltip("Current Workspace")

	quitItem := systray.AddMenuItem("Quit", "Quit the application")
	go func() {
		<-quitItem.ClickedCh
		systray.Quit()
	}()
}

func onExit() {
	if wsConn != nil {
		wsConn.Close()
	}
}

func loadIcon(name string) {
	relativePath := fmt.Sprintf("icons/%s.ico", name)
	iconPath, _ := filepath.Abs(relativePath)
	icon, err := os.ReadFile(iconPath)
	if err != nil {
		log.Println("Failed to load icon:", err)
	}

	systray.SetIcon(icon)
}

func connectWebSocket() {
	var err error
	wsConn, _, err = websocket.DefaultDialer.Dial("ws://localhost:6123", nil)
	if err != nil {
		log.Fatalf("Failed to connect to GlazeWM: %v", err)
		loadIcon("error")
	}
	defer wsConn.Close()

	fetchAndSetFocusedWorkspace()
	subscribeToEvents([]string{"focus_changed"})

	for {
		_, message, err := wsConn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			break
		}
		handleEvent(message)
	}
}

func subscribeToEvents(events []string) {
	subCmd := fmt.Sprintf("sub --events %s", events[0])
	for _, event := range events[1:] {
		subCmd += " " + event
	}
	wsConn.WriteMessage(websocket.TextMessage, []byte(subCmd))
}

func handleEvent(message []byte) {
	var event ServerMessage
	if err := json.Unmarshal(message, &event); err != nil {
		log.Printf("Error unmarshalling event: %v", err)
		return
	}

	if event.MessageType == "event_subscription" {
		fetchAndSetFocusedWorkspace()
	}
}

func queryMonitors() (map[string]interface{}, error) {
	mutex.Lock()
	defer mutex.Unlock()

	message := "query monitors"
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		return nil, fmt.Errorf("failed to send query: %w", err)
	}

	for {
		_, reply, err := wsConn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read reply: %w", err)
		}
		var response ServerMessage
		if err := json.Unmarshal(reply, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		if response.MessageType == "client_response" && response.ClientMessage == message {
			if response.Data != nil {
				if monitors, ok := response.Data.(map[string]interface{}); ok {
					return monitors, nil
				}
			}
		}
	}
}

func fetchAndSetFocusedWorkspace() {
	monitorsData, err := queryMonitors()
	if err != nil {
		log.Printf("Error fetching monitors: %v", err)
		loadIcon("default")
		return
	}

	monitors := monitorsData["monitors"].([]interface{})
	for _, monitor := range monitors {
		monitorMap := monitor.(map[string]interface{})
		if workspaces, ok := monitorMap["children"].([]interface{}); ok {
			for _, workspace := range workspaces {
				workspaceMap := workspace.(map[string]interface{})
				if hasFocus, _ := workspaceMap["hasFocus"].(bool); hasFocus {
					if name, ok := workspaceMap["name"].(string); ok {
						loadIcon(name)
						return
					}
				}
			}
		}
	}
	loadIcon("default")
}
