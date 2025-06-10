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
    // -----Send Message--------------------------------------------------------

    msg := mycelia.NewSendMessage("existing_route_name", "payload")
    err := mycelia.ProcessCommand(msg, address, port)
    if err != nil {
        fmt.Println("Failed to send command:", err)
    }

    // -----Add Channel---------------------------------------------------------

    ch := mycelia.NewAddChannel("existing_route_name", "new_channel_name")
    err := mycelia.ProcessCommand(ch, address, port)
    if err != nil {
        fmt.Println("Failed to send command:", err)
    }

    // -----Add Route-----------------------------------------------------------

    route := mycelia.NewAddRoute("new_route_name")
    err := mycelia.ProcessCommand(route, address, port)
    if err != nil {
        fmt.Println("Failed to send command:", err)
    }

    // -----Add Subscriber------------------------------------------------------

    subscribers_address = "127.0.0.1:5001"
    sub := mycelia.NewAddSubscriber("existing_route_name",
        "existing_channel_name", subscribers_address)
    err := mycelia.ProcessCommand(route, address, port)
    if err != nil {
        fmt.Println("Failed to send command:", err)
    }
}
```
