package ec2ic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/gorilla/websocket"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Dialer struct {
	cfg         aws.Config
	endpointId  string
	endpointDns string
	duration    time.Duration
}

func NewDialer(ctx context.Context, cfg aws.Config, endpointId string, duration time.Duration) (*Dialer, error) {
	api := ec2.NewFromConfig(cfg)

	if duration == 0 {
		duration = time.Hour
	}

	describe, err := api.DescribeInstanceConnectEndpoints(ctx, &ec2.DescribeInstanceConnectEndpointsInput{InstanceConnectEndpointIds: []string{endpointId}})
	if err != nil {
		return nil, fmt.Errorf("describing ec2 instance connect endpoint: %w", err)
	}

	return &Dialer{
		cfg:         cfg,
		endpointId:  endpointId,
		endpointDns: *describe.InstanceConnectEndpoints[0].DnsName,
		duration:    duration,
	}, nil
}

func (icd *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" {
		return nil, fmt.Errorf("only tcp supported")
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("splitting host and port: %w", err)
	}

	resolved, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolving private ip: %w", err)
	}

	q := url.Values{}
	q.Set("instanceConnectEndpointId", icd.endpointId)
	q.Set("maxTunnelDuration", fmt.Sprintf("%d", int(icd.duration.Seconds())))
	q.Set("remotePort", port)
	q.Set("privateIpAddress", resolved.String())

	r, _ := http.NewRequest("GET", fmt.Sprintf("wss://%s/openTunnel?%s", icd.endpointDns, q.Encode()), nil)

	sum := sha256.Sum256([]byte{})
	sumStr := hex.EncodeToString(sum[:])

	creds, err := icd.cfg.Credentials.Retrieve(ctx)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	s := v4.NewSigner()
	signed, _, err := s.PresignHTTP(ctx, creds, r, sumStr, "ec2-instance-connect", icd.cfg.Region, time.Now())
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, signed, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("dialing websocket: %w", err)
	}

	return &icdConn{
		Conn: conn,
		r:    websocket.JoinMessages(conn, ""),
	}, nil
}

type icdConn struct {
	*websocket.Conn
	r io.Reader
}

var _ net.Conn = (*icdConn)(nil)

func (i *icdConn) Read(b []byte) (n int, err error) {
	return i.r.Read(b)
}

func (i *icdConn) Write(b []byte) (n int, err error) {
	err = i.Conn.WriteMessage(websocket.BinaryMessage, b)
	n = len(b)
	return
}

func (i *icdConn) SetDeadline(t time.Time) error {
	i.SetReadDeadline(t)
	i.SetWriteDeadline(t)
	return nil
}
