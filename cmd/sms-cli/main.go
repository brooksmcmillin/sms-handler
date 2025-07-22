package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/brooksmcmillin/sms-handler"
)

type ChatUI struct {
	mu          sync.Mutex
	phoneNumber string
	smsHandler  *smshandler.SMSHandler
}

func NewChatUI(phoneNumber string, smsHandler *smshandler.SMSHandler) *ChatUI {
	return &ChatUI{
		phoneNumber: phoneNumber,
		smsHandler:  smsHandler,
	}
}

func (c *ChatUI) displayMessage(sender, message, timestamp string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear current line and move cursor up
	fmt.Print("\r\033[K")

	// Display the message
	fmt.Printf("%s [%s]: %s\n", sender, timestamp, message)

	// Redraw prompt
	fmt.Print("> ")
}

func (c *ChatUI) handleIncomingMessage(sms smshandler.SMS) {
	// Only show messages from our target phone number
	if sms.Sender == c.phoneNumber {
		c.displayMessage(sms.Sender, sms.Message, sms.Date)
	}
}

func (c *ChatUI) sendMessage(message string) error {
	if strings.TrimSpace(message) == "" {
		return nil
	}

	err := c.smsHandler.SendSMS(c.phoneNumber, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go sms.go <phone_number>")
		fmt.Println("Example: go run main.go sms.go +1234567890")
		os.Exit(1)
	}

	phoneNumber := os.Args[1]
	portName := "/dev/ttyUSB2"
	baudRate := 115200

	// Initialize SMS handler
	smsHandler, err := smshandler.NewSMSHandler(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to create SMS handler: %v", err)
	}
	defer func() {
		if err := smsHandler.Close(); err != nil {
			log.Printf("Error closing SMS handler: %v", err)
		}
	}()

	fmt.Printf("SMS Chat initialized successfully!\n")
	fmt.Printf("Connected to phone number: %s\n", phoneNumber)
	fmt.Printf("Type your messages and press Enter to send. Ctrl+C to exit.\n")
	fmt.Println(strings.Repeat("-", 50))

	// Create chat UI
	chat := NewChatUI(phoneNumber, smsHandler)

	// Start listening for incoming SMS messages
	smsHandler.ListenForIncomingSMS(chat.handleIncomingMessage)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		if err := smsHandler.Close(); err != nil {
			log.Printf("Error closing SMS handler: %v", err)
		}
		os.Exit(0)
	}()

	// Main input loop
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("> ")

	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		message := strings.TrimSpace(input)

		if message == "/quit" || message == "/exit" {
			break
		}

		if message == "/multi" {
			fmt.Println("Multi-line mode: Type your message, end with a line containing only '.' to send")
			var lines []string
			fmt.Print("| ")

			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				line = strings.TrimSpace(line)

				if line == "." {
					break
				}

				lines = append(lines, line)
				fmt.Print("| ")
			}

			if len(lines) > 0 {
				multiMessage := strings.Join(lines, "\n")
				if err := chat.sendMessage(multiMessage); err != nil {
					fmt.Printf("Error sending message: %v\n", err)
				}
			}
			fmt.Print("> ")
			continue
		}

		if message == "" {
			fmt.Print("> ")
			continue
		}

		if err := chat.sendMessage(message); err != nil {
			fmt.Printf("Error sending message: %v\n", err)
			fmt.Print("> ")
		} else {
			fmt.Print("> ")
		}
	}
}
