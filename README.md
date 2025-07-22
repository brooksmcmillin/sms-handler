# SMS Handler

A Go library for handling SMS messages through serial port communication with GSM modems.

## Installation

```bash
go get github.com/brooksmcmillin/sms-handler
```

## Library Usage

```go
package main

import (
    "log"
    "github.com/brooksmcmillin/sms-handler"
)

func main() {
    // Initialize the SMS handler
    smsHandler, err := smshandler.NewSMSHandler("/dev/ttyUSB2", 115200)
    if err != nil {
        log.Fatal(err)
    }
    defer smsHandler.Close()

    // Send an SMS
    err = smsHandler.SendSMS("+1234567890", "Hello, World!")
    if err != nil {
        log.Printf("Failed to send SMS: %v", err)
    }

    // Listen for incoming messages
    smsHandler.ListenForIncomingSMS(func(sms smshandler.SMS) {
        log.Printf("Received SMS from %s: %s", sms.Sender, sms.Message)
    })
}
```

## CLI Tool

The package includes a command-line chat interface for testing and recreational use.

### Installation

```bash
go install github.com/brooksmcmillin/sms-handler/cmd/sms-cli@latest
```

### Usage

```bash
sms-cli <phone_number>
```

Example:
```bash
sms-cli +1234567890
```

### CLI Features

- Real-time chat interface
- Send and receive SMS messages
- Multi-line message support (type `/multi`)
- Graceful shutdown with Ctrl+C
- Commands:
  - `/quit` or `/exit` - Exit the program
  - `/multi` - Enter multi-line mode (end with `.` on its own line)

## Examples

See the `examples/` directory for more usage examples.

### Usage Example
```
SMS Chat initialized successfully!
Connected to phone number: +11234567890
Type your messages and press Enter to send. Ctrl+C to exit.
--------------------------------------------------
> Hello!
+11234567890 [25/07/22]: Hi!
> 
```

## Requirements

- Go 1.18 or later
- A GSM modem connected via serial port
- Appropriate permissions to access the serial port

## Configuration

The default configuration uses:
- Port: `/dev/ttyUSB2`
- Baud Rate: `115200`

You can modify these values when creating a new SMS handler instance.
