package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/etf"
)

func create{{ .Name }}Handler() gen.TCPHandlerBehavior {
	return &{{ .Name }}Handler{}
}

type {{ .Name }}Handler struct {
	gen.TCPHandler
}

//
// Mandatory callbacks
//

// HandlePacket
func (th *{{ .Name }}Handler) HandlePacket(process *gen.TCPHandlerProcess, packet []byte, conn *gen.TCPConnection) (int, int, gen.TCPHandlerStatus) {
	fmt.Printf("[TCP handler] got message from %q: %#v\n", conn.Addr.String(), packet)

	// If you want to send a reply message, use conn.Socket.Write(reply) for that.

	// You may keep any data related to this connection in conn.State

	// To close connection use gen.TCPHandlerStatusClose on return

	// return values: left, await, status
	//        left   - how many bytes are left in the packet buffer (you might have
	//                 received a part of the next logical data).
	//        await  - what exact number of bytes you expect in the next packet
	//                 or leave it 0 if you are unsure.
	//        status - return gen.TCPHandlerStatusClose to close this connection

	// example:
	//   expected data of 5 bytes, but have received packet = []byte{1,2,3,4,5,6,7,8}
	//   you must return 3, 5, gen.TCPHandlerStatusOK
	//   So the following invocation will happen after receiving two more bytes
	//   The next packet will have []byte{6,7,8,9,0}

	return 0, 0, gen.TCPHandlerStatusOK
}

//
// Optional callbacks
//

func (th *{{ .Name }}Handler) HandleConnect(process *gen.TCPHandlerProcess, conn *gen.TCPConnection) gen.TCPHandlerStatus {
	fmt.Printf("[TCP handler] got new connection from %q\n", conn.Addr.String())
	return gen.TCPHandlerStatusOK
}
func (th *{{ .Name }}Handler) HandleDisconnect(process *gen.TCPHandlerProcess, conn *gen.TCPConnection) {
	fmt.Printf("[TCP handler] connection with %q terminated\n", conn.Addr.String())
}

func (th *{{ .Name }}Handler) HandleTimeout(process *gen.TCPHandlerProcess, conn *gen.TCPConnection) gen.TCPHandlerStatus {
	return gen.TCPHandlerStatusOK
}

//
// Optional gen.Server's callbacks
//

// HandleTCPHandlerCall this callback is invoked on ServerProcess.Call(...).
func (r *{{ .Name }}Handler) HandleTCPHandlerCall(process *gen.TCPHandlerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleTCPHandlerCast this callback is invoked on ServerProcess.Cast(...).
func (r *{{ .Name }}Handler) HandleTCPHandlerCast(process *gen.TCPHandlerProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleTCPHandlerInfo this callback is invoked on Process.Send(...).
func (r *{{ .Name }}Handler) HandleTCPHandlerInfo(process *gen.TCPHandlerProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleTCPHandlerTerminate this callback is invoked on the process termiation
func (r *{{ .Name }}Handler) HandleTCPHandlerTerminate(process *gen.TCPHandlerProcess, reason string) {

}
