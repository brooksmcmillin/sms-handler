package main

import (
	"fmt"
	"log"

	"github.com/brooksmcmillin/sms-handler"
)

func main() {
	// Initialize SMS handler
	// Replace with your actual serial port and baud rate
	portName := "/dev/ttyUSB2"
	baudRate := 115200

	smsHandler, err := smshandler.NewSMSHandler(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to create SMS handler: %v", err)
	}
	defer smsHandler.Close()

	// Example 1: Send an SMS
	phoneNumber := "+1234567890" // Replace with actual phone number
	message := "Hello from sms-handler library!"
	
	fmt.Printf("Sending SMS to %s...\n", phoneNumber)
	err = smsHandler.SendSMS(phoneNumber, message)
	if err != nil {
		log.Printf("Failed to send SMS: %v", err)
	} else {
		fmt.Println("SMS sent successfully!")
	}

	// Example 2: Listen for incoming SMS messages
	fmt.Println("\nListening for incoming SMS messages (press Ctrl+C to stop)...")
	
	// Define a callback function for incoming messages
	messageHandler := func(sms smshandler.SMS) {
		fmt.Printf("\nReceived SMS:\n")
		fmt.Printf("  From: %s\n", sms.Sender)
		fmt.Printf("  Date: %s\n", sms.Date)
		fmt.Printf("  Message: %s\n", sms.Message)
		fmt.Println()
	}

	// Start listening for incoming messages
	smsHandler.ListenForIncomingSMS(messageHandler)

	// Keep the program running
	select {}
}