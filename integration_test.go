// +build integration

package smshandler

import (
	"os"
	"testing"
	"time"
)

// Integration tests require a real modem connected
// Run with: go test -tags=integration

func TestIntegrationSendReceiveSMS(t *testing.T) {
	// Skip if not in integration mode
	if os.Getenv("SMS_TEST_PORT") == "" || os.Getenv("SMS_TEST_PHONE") == "" {
		t.Skip("Skipping integration test. Set SMS_TEST_PORT and SMS_TEST_PHONE env vars to run.")
	}
	
	port := os.Getenv("SMS_TEST_PORT")       // e.g., "/dev/ttyUSB2"
	testPhone := os.Getenv("SMS_TEST_PHONE") // e.g., "+1234567890"
	
	// Create handler
	handler, err := NewSMSHandler(port, 115200)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}
	defer handler.Close()
	
	// Test 1: Send SMS
	t.Run("SendSMS", func(t *testing.T) {
		testMessage := "Integration test message: " + time.Now().Format("15:04:05")
		err := handler.SendSMS(testPhone, testMessage)
		if err != nil {
			t.Errorf("Failed to send SMS: %v", err)
		}
	})
	
	// Test 2: Receive SMS
	t.Run("ReceiveSMS", func(t *testing.T) {
		receivedChan := make(chan SMS, 1)
		
		// Set up listener
		handler.ListenForIncomingSMS(func(sms SMS) {
			select {
			case receivedChan <- sms:
			default:
			}
		})
		
		// Wait for message or timeout
		select {
		case sms := <-receivedChan:
			t.Logf("Received SMS from %s: %s", sms.Sender, sms.Message)
		case <-time.After(30 * time.Second):
			t.Log("No SMS received within timeout (this is normal if no test SMS was sent)")
		}
	})
}

func TestIntegrationModemInfo(t *testing.T) {
	if os.Getenv("SMS_TEST_PORT") == "" {
		t.Skip("Skipping integration test. Set SMS_TEST_PORT env var to run.")
	}
	
	port := os.Getenv("SMS_TEST_PORT")
	
	handler, err := NewSMSHandler(port, 115200)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}
	defer handler.Close()
	
	// Test various AT commands
	tests := []struct {
		name    string
		command string
	}{
		{"Manufacturer", "AT+CGMI"},
		{"Model", "AT+CGMM"},
		{"IMEI", "AT+CGSN"},
		{"Signal Quality", "AT+CSQ"},
		{"Network Registration", "AT+CREG?"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := handler.sendATCommand(tt.command)
			if err != nil {
				t.Errorf("Command %s failed: %v", tt.command, err)
				return
			}
			t.Logf("%s response: %s", tt.name, resp)
		})
	}
}