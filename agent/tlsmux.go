package agent

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"sync"
)

// ErrPlainHTTPRefused: llegó HTTP sin cifrar a un listener con
// allowPlain=false (`vigia-agent run -http-legacy=false`).
var ErrPlainHTTPRefused = errors.New("HTTP sin cifrar desactivado (-http-legacy=false)")

// tlsRecordHandshake: primer byte de todo ClientHello (registro TLS de tipo
// handshake). Una petición HTTP empieza con una letra ("GET", "POST"…), así
// que un solo byte alcanza para distinguirlas.
const tlsRecordHandshake = 0x16

// NewDualListener sirve HTTPS y, si allowPlain, también HTTP sin cifrar en
// el MISMO puerto (TLS v1.5, ver docs/agente-api.md): así una app de antes
// de esta fase, que habla HTTP al 7341 de siempre, sigue funcionando hasta
// que se actualiza y fija la huella — sin un segundo puerto que anunciar ni
// recordar. Cada conexión decide en su primera lectura (ver dualConn), no en
// Accept: un cliente lento no frena a los demás.
func NewDualListener(inner net.Listener, cfg *tls.Config, allowPlain bool) net.Listener {
	return &dualListener{Listener: inner, cfg: cfg, allowPlain: allowPlain}
}

type dualListener struct {
	net.Listener
	cfg        *tls.Config
	allowPlain bool
}

func (l *dualListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &dualConn{Conn: c, cfg: l.cfg, allowPlain: l.allowPlain}, nil
}

// dualConn mira el primer byte en la primera lectura o escritura y desde ahí
// delega en un *tls.Conn o en la conexión tal cual. Los plazos
// (SetReadDeadline…) van siempre a la conexión de abajo, que es donde los
// pone también tls.Conn: el ReadHeaderTimeout de http.Server cubre la espera
// del primer byte y el handshake.
type dualConn struct {
	net.Conn
	cfg        *tls.Config
	allowPlain bool

	once  sync.Once
	inner net.Conn
	err   error
}

func (c *dualConn) init() {
	c.once.Do(func() {
		br := bufio.NewReader(c.Conn)
		first, err := br.Peek(1)
		if err != nil {
			c.err = err
			return
		}
		peeked := &peekedConn{Conn: c.Conn, r: br}
		switch {
		case first[0] == tlsRecordHandshake:
			c.inner = tls.Server(peeked, c.cfg)
		case c.allowPlain:
			c.inner = peeked
		default:
			c.err = ErrPlainHTTPRefused
		}
	})
}

func (c *dualConn) Read(p []byte) (int, error) {
	c.init()
	if c.err != nil {
		return 0, c.err
	}
	return c.inner.Read(p)
}

func (c *dualConn) Write(p []byte) (int, error) {
	c.init()
	if c.err != nil {
		return 0, c.err
	}
	return c.inner.Write(p)
}

// Close cierra la conexión de abajo directamente (sin el close_notify de
// TLS): puede llegar desde otra goroutine (Shutdown) antes de init.
func (c *dualConn) Close() error { return c.Conn.Close() }

// peekedConn devuelve primero lo que bufio ya leyó al mirar el primer byte.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
