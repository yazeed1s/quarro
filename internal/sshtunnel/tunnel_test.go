package sshtunnel

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// testServer is a minimal, disposable SSH server: it accepts a password or
// a public key (whichever the caller configures), and proxies any
// "direct-tcpip" channel (what ssh.Client.Dial opens) straight to the real
// TCP address the client asked for. That is enough to exercise Open and
// DialContext end to end without a real network or Docker.
type testServer struct {
	addr   string
	hostFP string
}

func startTestServer(t *testing.T, password string, authorizedKey ssh.PublicKey) *testServer {
	t.Helper()
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if password != "" && string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("wrong password")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if authorizedKey != nil && bytes.Equal(key.Marshal(), authorizedKey.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown public key")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			nc, e := ln.Accept()
			if e != nil {
				return
			}
			go serveConn(nc, cfg)
		}
	}()

	pub, e := ssh.NewPublicKey(hostKey.Public())
	if e != nil {
		t.Fatalf("host public key: %v", e)
	}
	return &testServer{addr: ln.Addr().String(), hostFP: ssh.FingerprintSHA256(pub)}
}

// serveConn handles one SSH client: after the handshake, it forwards every
// direct-tcpip channel to the address the client requested, byte for byte
// in both directions.
func serveConn(nc net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, e := ssh.NewServerConn(nc, cfg)
	if e != nil {
		nc.Close()
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "direct-tcpip" {
			nch.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		var hdr struct {
			DestAddr string
			DestPort uint32
			SrcAddr  string
			SrcPort  uint32
		}
		if e := ssh.Unmarshal(nch.ExtraData(), &hdr); e != nil {
			nch.Reject(ssh.ConnectionFailed, "bad request")
			continue
		}
		ch, reqs, e := nch.Accept()
		if e != nil {
			continue
		}
		go ssh.DiscardRequests(reqs)
		go forward(ch, net.JoinHostPort(hdr.DestAddr, portStr(hdr.DestPort)))
	}
}

func forward(ch ssh.Channel, target string) {
	dst, e := net.Dial("tcp", target)
	if e != nil {
		ch.Close()
		return
	}
	go func() {
		io.Copy(dst, ch)
		dst.Close()
	}()
	io.Copy(ch, dst)
	ch.Close()
}

func portStr(p uint32) string {
	const digits = "0123456789"
	if p == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for p > 0 {
		i--
		buf[i] = digits[p%10]
		p /= 10
	}
	return string(buf[i:])
}

// echoServer is the "database" DialContext proxies to: it echoes back
// whatever it reads, so a successful tunnel round-trip is observable.
func startEchoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			go func() {
				io.Copy(c, c)
				c.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

func splitHostPort(t *testing.T, addr string) (string, string) {
	t.Helper()
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	return h, p
}

func TestOpenPasswordAuth(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tun.Close()
	if tun.Fingerprint() != srv.hostFP {
		t.Fatalf("Fingerprint() = %q, want %q", tun.Fingerprint(), srv.hostFP)
	}
}

func TestOpenWrongPassword(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	_, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "wrong",
	})
	if err == nil {
		t.Fatal("Open succeeded with the wrong password")
	}
}

func TestOpenKeyAuth(t *testing.T) {
	pub, priv := generateKey(t, "")
	srv := startTestServer(t, "", pub)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "key", PrivateKeyPEM: priv,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tun.Close()
}

func TestOpenKeyAuthWithPassphrase(t *testing.T) {
	pub, priv := generateKey(t, "correct horse")
	srv := startTestServer(t, "", pub)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "key", PrivateKeyPEM: priv, Passphrase: "correct horse",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tun.Close()
}

func TestOpenWrongPassphrase(t *testing.T) {
	pub, priv := generateKey(t, "correct horse")
	srv := startTestServer(t, "", pub)
	host, port := splitHostPort(t, srv.addr)
	_, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "key", PrivateKeyPEM: priv, Passphrase: "wrong",
	})
	if err == nil {
		t.Fatal("Open succeeded with the wrong passphrase")
	}
}

func TestOpenHostKeyMismatchRejected(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	_, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "hunter2",
		HostFingerprint: "SHA256:not-the-real-fingerprint",
	})
	if err == nil {
		t.Fatal("Open succeeded despite a mismatched trusted host key")
	}
}

func TestOpenHostKeyMatchAccepted(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "hunter2",
		HostFingerprint: srv.hostFP,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tun.Close()
}

func TestDialContextProxiesData(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tun.Close()

	echoAddr := startEchoServer(t)
	c, err := tun.DialContext(context.Background(), "tcp", echoAddr)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer c.Close()

	want := []byte("hello through the tunnel")
	if _, err := c.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := make([]byte, len(want))
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// A driver that calls SetDeadline unconditionally on a dialed connection
// (go-sql-driver/mysql does) must not see an error: ssh.Client.Dial's own
// connection type rejects every deadline call outright, so DialContext must
// wrap it.
func TestDialContextConnAcceptsDeadlineCalls(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tun.Close()

	echoAddr := startEchoServer(t)
	c, err := tun.DialContext(context.Background(), "tcp", echoAddr)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if err := c.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if err := c.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
}

func TestCloseStopsFurtherDials(t *testing.T) {
	srv := startTestServer(t, "hunter2", nil)
	host, port := splitHostPort(t, srv.addr)
	tun, err := Open(context.Background(), Config{
		Host: host, Port: port, Username: "u",
		AuthMethod: "password", Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := tun.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	echoAddr := startEchoServer(t)
	done := make(chan error, 1)
	go func() {
		_, e := tun.DialContext(context.Background(), "tcp", echoAddr)
		done <- e
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("DialContext succeeded after Close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DialContext hung after Close instead of failing")
	}
}

// generateKey returns an ed25519 public key and a PEM-encoded private key,
// optionally passphrase-encrypted, for the "key" auth-method tests.
func generateKey(t *testing.T, passphrase string) (ssh.PublicKey, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	var block *pem.Block
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(priv, "")
	}
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return sshPub, pem.EncodeToMemory(block)
}
