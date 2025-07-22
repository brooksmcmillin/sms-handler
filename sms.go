package smshandler

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

type SMSHandler struct {
	port       serial.Port
	reader     *bufio.Reader
	readerMu   sync.Mutex
	listening  bool
	pauseChan  chan bool
	resumeChan chan bool
}

type SMS struct {
	Index   int
	Status  string
	Sender  string
	Date    string
	Message string
}

func readUntilAny(r *bufio.Reader, delimiters []byte) (string, byte, error) {
	// Reads up to and including the delimiter
	var result []byte
	delimSet := make(map[byte]bool)
	for _, d := range delimiters {
		delimSet[d] = true
	}

	for {
		b, err := r.ReadByte()
		if err != nil {
			return string(result), 0, err
		}

		result = append(result, b)
		if delimSet[b] {
			return string(result), b, nil
		}
	}
}

func NewSMSHandler(portName string, baudRate int) (*SMSHandler, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port: %v", err)
	}

	handler := &SMSHandler{
		port:       port,
		reader:     bufio.NewReader(port),
		pauseChan:  make(chan bool),
		resumeChan: make(chan bool),
	}

	// Initialize Modem
	if err := handler.initModem(); err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to instantiate modem: %v", err)
	}

	return handler, nil
}

// Close connection
func (s *SMSHandler) Close() error {
	return s.port.Close()
}

// pauseListener temporarily pauses the SMS listener
func (s *SMSHandler) pauseListener() {
	if s.listening {
		s.pauseChan <- true
		// Wait for confirmation that listener is paused
		<-s.resumeChan
	}
}

// resumeListener resumes the SMS listener
func (s *SMSHandler) resumeListener() {
	if s.listening {
		s.resumeChan <- true
	}
}

// sendATCommand sends an AT command and waits for response
func (s *SMSHandler) sendATCommand(command string) (string, error) {
	s.pauseListener()
	defer s.resumeListener()

	// Clear any pending data in the buffer
	for s.reader.Buffered() > 0 {
		s.reader.ReadByte()
	}

	// Send command
	_, err := s.port.Write([]byte(command + "\r\n"))
	if err != nil {
		return "", fmt.Errorf("failed to write command: %v", err)
	}

	// Read response with timeout
	response := ""
	timeout := time.After(10 * time.Second)
	done := make(chan bool)

	go func() {
		consecutiveEmpty := 0
		for {
			line, err := s.reader.ReadString('\n')
			if err != nil {
				done <- true
				break
			}

			line = strings.TrimSpace(line)

			// Skip echo of the command itself
			if line == command {
				continue
			}

			// Skip empty lines but track them
			if line == "" {
				consecutiveEmpty++
				if consecutiveEmpty > 3 {
					// Too many empty lines, might be stuck
					done <- true
					break
				}
				continue
			}
			consecutiveEmpty = 0

			response += line + "\n"

			// Check for terminal responses
			if strings.Contains(line, "OK") || strings.Contains(line, "ERROR") || strings.Contains(line, "+CME ERROR") {
				done <- true
				break
			}
		}
	}()

	select {
	case <-done:
		return strings.TrimSpace(response), nil
	case <-timeout:
		// Try to get whatever we have so far
		return strings.TrimSpace(response), fmt.Errorf("command timeout")
	}
}

// initModem initializes the modem with basic AT commands
func (s *SMSHandler) initModem() error {
	// Test AT communication
	if _, err := s.sendATCommand("AT"); err != nil {
		return fmt.Errorf("AT test failed: %v", err)
	}

	// Set text mode for SMS
	if _, err := s.sendATCommand("AT+CMGF=1"); err != nil {
		return fmt.Errorf("failed to set SMS text mode: %v", err)
	}

	// Set character set to GSM
	if _, err := s.sendATCommand("AT+CSCS=\"GSM\""); err != nil {
		return fmt.Errorf("failed to set character set: %v", err)
	}

	// Configure SMS storage location (SIM card)
	if _, err := s.sendATCommand("AT+CPMS=\"SM\",\"SM\",\"SM\""); err != nil {
		return fmt.Errorf("failed to set SMS storage: %v", err)
	}

	// Enable SMS delivery notifications - try different settings for compatibility
	_, err := s.sendATCommand("AT+CNMI=1,2,0,1,0")
	if err != nil {
		_, err = s.sendATCommand("AT+CNMI=2,1,0,2,0")
		if err != nil {
			_, err = s.sendATCommand("AT+CNMI=1,1,0,1,0")
			if err != nil {
				return fmt.Errorf("failed to enable SMS notifications: %v", err)
			}
		}
	}

	return nil
}

func (s *SMSHandler) GetModemInfo() (string, error) {
	return s.sendATCommand("ATI")
}

// GetSignalStrength returns signal strength information
func (s *SMSHandler) GetSignalStrength() (string, error) {
	return s.sendATCommand("AT+CSQ")
}

// ReadSMS reads all SMS messages
func (s *SMSHandler) ReadSMS() ([]SMS, error) {
	response, err := s.sendATCommand("AT+CMGL=\"ALL\"")
	if err != nil {
		return nil, fmt.Errorf("failed to read SMS: %v", err)
	}

	return s.parseSMSList(response), nil
}

// ReadNewSMS reads only unread SMS messages
func (s *SMSHandler) ReadNewSMS() ([]SMS, error) {
	response, err := s.sendATCommand("AT+CMGL=\"REC UNREAD\"")
	if err != nil {
		return nil, fmt.Errorf("failed to read new SMS: %v", err)
	}

	return s.parseSMSList(response), nil
}

// parseSMSList parses the response from AT+CMGL command
func (s *SMSHandler) parseSMSList(response string) []SMS {
	var messages []SMS
	lines := strings.Split(response, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "+CMGL:") {
			// Parse header line: +CMGL: index,status,sender,date
			parts := strings.Split(line, ",")
			if len(parts) >= 4 {
				var sms SMS
				fmt.Sscanf(parts[0], "+CMGL: %d", &sms.Index)
				sms.Status = strings.Trim(parts[1], "\"")
				sms.Sender = strings.Trim(parts[2], "\"")
				sms.Date = strings.Trim(parts[3], "\"")

				// Next line should contain the message
				if i+1 < len(lines) {
					sms.Message = strings.TrimSpace(lines[i+1])
					i++ // Skip the message line in next iteration
				}
				messages = append(messages, sms)
			}
		}
	}

	return messages
}

// DeleteSMS deletes an SMS message by index
func (s *SMSHandler) DeleteSMS(index int) error {
	cmd := fmt.Sprintf("AT+CMGD=%d", index)
	_, err := s.sendATCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to delete SMS: %v", err)
	}
	return nil
}

// ListenForIncomingSMS listens for incoming SMS notifications
func (s *SMSHandler) ListenForIncomingSMS(callback func(SMS)) {
	s.listening = true
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("SMS listener recovered from panic: %v\n", r)
			}
		}()

		for s.listening {
			select {
			case <-s.pauseChan:
				// Listener paused, confirm and wait for resume
				s.resumeChan <- true
				<-s.resumeChan
			default:
				// Check if there's data available to read
				s.port.SetReadTimeout(100 * time.Millisecond)

				// Read line by line to properly handle multi-line messages
				line, err := s.reader.ReadString('\n')
				if err == nil {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}

					// Filter out AT command responses and other non-SMS lines
					if s.isATResponse(line) {
						continue
					}

					// Check for direct SMS delivery: +CMT: "sender","","date"
					if strings.HasPrefix(line, "+CMT:") {
						s.handleCMTMessage(line, callback)
					}

					// Also check for stored message notifications: +CMTI: "SM",index
					if strings.HasPrefix(line, "+CMTI:") {
						s.handleCMTIMessage(line, callback)
					}
				}
			}
		}
	}()
}

// isATResponse checks if a line is an AT command or response that should be filtered out
func (s *SMSHandler) isATResponse(line string) bool {
	// Only filter out lines that are clearly AT commands (not responses we might need)
	if strings.HasPrefix(line, "AT+") || line == "AT" {
		return true
	}

	// Filter out some specific responses that are never SMS content
	if line == "OK" || line == "ERROR" {
		return true
	}

	// Filter out status responses that start with +
	if strings.HasPrefix(line, "+CMGF:") ||
		strings.HasPrefix(line, "+CSCS:") ||
		strings.HasPrefix(line, "+CPMS:") ||
		strings.HasPrefix(line, "+CNMI:") ||
		strings.HasPrefix(line, "+CSQ:") {
		return true
	}

	return false
}

// handleCMTMessage handles direct SMS delivery notifications
func (s *SMSHandler) handleCMTMessage(line string, callback func(SMS)) {
	// Parse CMT header: +CMT: "+11234567890","","25/07/21,21:07:17-28"
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return
	}

	var sms SMS

	// Extract sender from first part, removing "+CMT: " prefix safely
	senderPart := parts[0]
	if len(senderPart) > 6 { // "+CMT: " is 6 characters
		sms.Sender = strings.Trim(senderPart[6:], "\"")
	} else {
		return // Invalid format
	}

	// Extract date from last part
	if len(parts) >= 3 {
		sms.Date = strings.Trim(parts[2], "\"")
	}

	// Now read the actual message content that follows the header
	// The message comes after the +CMT line
	s.readerMu.Lock()
	defer s.readerMu.Unlock()

	// Read the message content
	messageLines := []string{}
	timeout := time.After(2 * time.Second)

	for {
		select {
		case <-timeout:
			// If we timeout, use what we have
			if len(messageLines) > 0 {
				sms.Message = strings.Join(messageLines, "\n")
				callback(sms)
			}
			return
		default:
			// Try to read a line
			s.port.SetReadTimeout(100 * time.Millisecond)
			line, err := s.reader.ReadString('\n')
			if err == nil {
				line = strings.TrimSpace(line)

				// Skip empty lines at the beginning
				if line == "" && len(messageLines) == 0 {
					continue
				}

				// Check if this is the end of the message or another notification
				if strings.HasPrefix(line, "+CMT:") || strings.HasPrefix(line, "+CMTI:") ||
					strings.HasPrefix(line, "OK") || strings.HasPrefix(line, "ERROR") ||
					strings.HasPrefix(line, "AT+") {
					// We've hit the next command/notification, so we're done
					if len(messageLines) > 0 {
						sms.Message = strings.Join(messageLines, "\n")
						callback(sms)
					}
					return
				}

				// This is part of the message
				if line != "" {
					messageLines = append(messageLines, line)
				} else if len(messageLines) > 0 {
					// Empty line after we've started collecting message - we're done
					sms.Message = strings.Join(messageLines, "\n")
					callback(sms)
					return
				}
			}
		}
	}
}

// handleCMTIMessage handles stored message notifications
func (s *SMSHandler) handleCMTIMessage(line string, callback func(SMS)) {
	parts := strings.Split(line, ",")
	if len(parts) >= 2 {
		var index int
		fmt.Sscanf(parts[1], "%d", &index)

		// Read the specific SMS message
		sms, err := s.readSMSByIndex(index)
		if err == nil {
			callback(sms)
		}
	}
}

// readSMSByIndex reads a specific SMS message by index
func (s *SMSHandler) readSMSByIndex(index int) (SMS, error) {
	cmd := fmt.Sprintf("AT+CMGR=%d", index)
	response, err := s.sendATCommand(cmd)
	if err != nil {
		return SMS{}, fmt.Errorf("failed to read SMS: %v", err)
	}

	lines := strings.Split(response, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CMGR:") {
			// Parse header line: +CMGR: status,sender,date
			parts := strings.Split(line, ",")
			if len(parts) >= 3 {
				var sms SMS
				sms.Index = index
				sms.Status = strings.Trim(parts[0][7:], "\"") // Remove "+CMGR: "
				sms.Sender = strings.Trim(parts[1], "\"")
				sms.Date = strings.Trim(parts[2], "\"")

				// Next line should contain the message
				if i+1 < len(lines) {
					sms.Message = strings.TrimSpace(lines[i+1])
				}
				return sms, nil
			}
		}
	}

	return SMS{}, fmt.Errorf("failed to parse SMS")
}

func (s *SMSHandler) SendSMS(phoneNumber, message string) error {
	s.pauseListener()
	defer s.resumeListener()

	// Clear any pending data in the buffer
	for s.reader.Buffered() > 0 {
		s.reader.ReadByte()
	}

	// Small delay to ensure modem is ready
	time.Sleep(100 * time.Millisecond)

	// Start SMS composition
	cmd := fmt.Sprintf("AT+CMGS=\"%s\"", phoneNumber)
	// fmt.Printf("Sending command: %s\n", cmd)

	// Send the AT+CMGS command with just CR
	_, err := s.port.Write([]byte(cmd + "\r"))
	if err != nil {
		return fmt.Errorf("failed to write AT+CMGS command: %v", err)
	}

	// Wait for response and '>' prompt
	promptBuffer := make([]byte, 0, 256)
	promptReceived := false
	startTime := time.Now()

	for !promptReceived && time.Since(startTime) < 10*time.Second {
		// Set a short read timeout
		s.port.SetReadTimeout(100 * time.Millisecond)

		buf := make([]byte, 1)
		n, err := s.port.Read(buf)
		if err == nil && n > 0 {
			promptBuffer = append(promptBuffer, buf[0])
			// fmt.Printf("Read: %d ('%c') | Buffer: %q\n", buf[0], buf[0], string(promptBuffer))

			// Check if we've received the '>' prompt
			if bytes.Contains(promptBuffer, []byte(">")) {
				promptReceived = true
				// fmt.Println("Prompt received!")
			}
		}
	}

	if !promptReceived {
		return fmt.Errorf("timeout waiting for SMS prompt, got: %q", string(promptBuffer))
	}

	// Small delay after prompt
	time.Sleep(100 * time.Millisecond)

	// fmt.Printf("Sending message: %s\n", message)

	// Send message content followed by Ctrl+Z
	fullMessage := message + "\x1A" // \x1A is Ctrl+Z
	_, err = s.port.Write([]byte(fullMessage))
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}

	// fmt.Println("Message sent with Ctrl+Z, waiting for response...")

	// Read response
	responseBuffer := make([]byte, 0, 1024)
	startTime = time.Now()

	for time.Since(startTime) < 30*time.Second {
		s.port.SetReadTimeout(100 * time.Millisecond)

		buf := make([]byte, 128)
		n, err := s.port.Read(buf)
		if err == nil && n > 0 {
			responseBuffer = append(responseBuffer, buf[:n]...)
			response := string(responseBuffer)
			// fmt.Printf("Response so far: %q\n", response)

			// Check for completion
			if strings.Contains(response, "+CMGS:") || strings.Contains(response, "OK") {
				if strings.Contains(response, "+CMGS:") {
					return nil
				}
			}
			if strings.Contains(response, "ERROR") || strings.Contains(response, "+CMS ERROR") {
				return fmt.Errorf("SMS failed: %s", response)
			}
		}
	}

	return fmt.Errorf("SMS timeout - no valid response received")
}
