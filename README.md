# Mycelium-Go-API
Mycelium Golang API

## Usage

Define a command type and then call `process_command(cmd)`.

### Example

```go
package main

import "https://github.com/SignalWeave/Mycelia-Go-API"

var address string = "127.0.0.1"
var port int = 5000


func main() {
    // Start a simple echo transformer
    listener := NewMyceliaListener(
        func(p []byte) []byte {
            return append([]byte("X:"), p...)
            },
            "127.0.0.1",
            7010,
        )

    go func() { _ = listener.Start() }()
    defer listener.Stop()
}

func sendAndRecv() {
    response := Send(
        Message{
            SenderAddress: GetLocalIPv4() + ":7777",
            Route: "default",
            Payload: []byte("hello")
            },
        "127.0.0.1",
        5500,
    )
    fmt.Println(response.Ack)
    fmt.Println(response.UID)
}
```
