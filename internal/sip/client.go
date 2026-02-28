package sip

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"

	"github.com/darrenwiebe/teams_freepbx/internal/blf"
)

// Config holds SIP endpoint and auth settings.
type Config struct {
	Server      string   // host:port
	Transport   string   // UDP, TCP, etc.
	Username    string
	Password    string
	ContactIP   string   // our IP for Contact header; use "auto" or leave empty for STUN discovery
	ContactPort int      // port for Contact (0 = 5060 or omit); set by STUN when behind NAT
	STUNServers []string // STUN servers for NAT discovery (e.g. stun.l.google.com)
	UserAgent   string
}

// BLFHandler is called when a BLF state change is received (extension, state).
type BLFHandler func(extension string, state blf.State)

// Client registers to a SIP server and subscribes to BLF (dialog) for a list of extensions.
type Client struct {
	ua     *sipgo.UserAgent
	client *sipgo.Client
	server *sipgo.Server
	cfg    Config
	extensions []string
	onBLF  BLFHandler
	log    *slog.Logger
	mu     sync.Mutex
}

// serverHost returns the host part of cfg.Server (no port) for use in From header.
func serverHost(server string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(server))
	if err != nil {
		return strings.TrimSpace(server)
	}
	return host
}

// NewClient creates a SIP client. Call Register then Subscribe; run the server to handle NOTIFY.
// cfg.ContactIP and cfg.ContactPort should already be set (e.g. from STUN when behind NAT).
// The UA identity (From header) is set to cfg.Username@serverHost so the PBX can match the registered peer.
func NewClient(cfg Config, extensions []string, onBLF BLFHandler) (*Client, error) {
	host := serverHost(cfg.Server)
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(cfg.Username),
		sipgo.WithUserAgentHostname(host),
	)
	if err != nil {
		return nil, err
	}
	opts := []sipgo.ClientOption{sipgo.WithClientHostname(cfg.ContactIP)}
	if cfg.ContactPort > 0 {
		opts = append(opts, sipgo.WithClientPort(cfg.ContactPort), sipgo.WithClientNAT())
	}
	client, err := sipgo.NewClient(ua, opts...)
	if err != nil {
		ua.Close()
		return nil, err
	}
	server, err := sipgo.NewServer(ua)
	if err != nil {
		client.Close()
		ua.Close()
		return nil, err
	}
	c := &Client{
		ua:         ua,
		client:     client,
		server:     server,
		cfg:        cfg,
		extensions: extensions,
		onBLF:     onBLF,
		log:        slog.Default().With("component", "sip"),
	}
	server.OnNotify(c.handleNOTIFY)
	return c, nil
}

// Close shuts down the client and UA.
func (c *Client) Close() error {
	c.client.Close()
	return c.ua.Close()
}

// ListenAndServe starts the SIP server listening for NOTIFYs. Call in a goroutine or block.
func (c *Client) ListenAndServe(ctx context.Context, network, addr string) error {
	return c.server.ListenAndServe(ctx, network, addr)
}

// Register sends REGISTER and handles 401 with digest auth.
func (c *Client) Register(ctx context.Context) error {
	recipient := sip.Uri{}
	parseURI := fmt.Sprintf("sip:%s@%s", c.cfg.Username, c.cfg.Server)
	if err := sip.ParseUri(parseURI, &recipient); err != nil {
		return err
	}
	req := sip.NewRequest(sip.REGISTER, recipient)
	req.AppendHeader(sip.NewHeader("Contact", c.contactAddr()))
	req.SetTransport(strings.ToUpper(c.cfg.Transport))

	tx, err := c.client.TransactionRequest(ctx, req, sipgo.ClientRequestRegisterBuild)
	if err != nil {
		return err
	}
	defer tx.Terminate()

	res, err := c.getResponse(tx)
	if err != nil {
		return err
	}

	if res.StatusCode == 401 {
		wwwAuth := res.GetHeader("WWW-Authenticate")
		if wwwAuth == nil {
			return fmt.Errorf("401 without WWW-Authenticate")
		}
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
			return err
		}
		cred, err := digest.Digest(chal, digest.Options{
			Method:   req.Method.String(),
			URI:      recipient.Host,
			Username: c.cfg.Username,
			Password: c.cfg.Password,
		})
		if err != nil {
			return err
		}
		newReq := req.Clone()
		newReq.RemoveHeader("Via")
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))
		tx2, err := c.client.TransactionRequest(ctx, newReq, sipgo.ClientRequestIncreaseCSEQ, sipgo.ClientRequestAddVia)
		if err != nil {
			return err
		}
		defer tx2.Terminate()
		res, err = c.getResponse(tx2)
		if err != nil {
			return err
		}
	}

	if res.StatusCode != 200 && res.StatusCode != 202 {
		return fmt.Errorf("register failed: %d", res.StatusCode)
	}
	c.log.Info("registered", "status", res.StatusCode)
	return nil
}

// Subscribe sends SUBSCRIBE for the dialog event package for each extension.
// Continues on 404 so other extensions can still be subscribed; returns error only if all fail.
func (c *Client) Subscribe(ctx context.Context) error {
	var failed []string
	for _, ext := range c.extensions {
		if err := c.subscribeOne(ctx, ext); err != nil {
			if strings.Contains(err.Error(), "404") {
				c.log.Warn("subscribe 404 (extension may lack BLF hint on PBX)", "extension", ext, "hint", "See README or FreePBX dialplan hints / res_pjsip allow_subscribe")
			} else {
				c.log.Error("subscribe failed", "extension", ext, "error", err)
			}
			failed = append(failed, ext)
			continue
		}
		c.log.Info("subscribed to BLF", "extension", ext)
	}
	if len(failed) == len(c.extensions) {
		return fmt.Errorf("all subscriptions failed (extensions: %v); check PBX dialplan hints and res_pjsip allow_subscribe", failed)
	}
	if len(failed) > 0 {
		c.log.Warn("some extensions could not be subscribed", "failed", failed)
	}
	return nil
}

func (c *Client) subscribeOne(ctx context.Context, extension string) error {
	recipient := sip.Uri{}
	parseURI := fmt.Sprintf("sip:%s@%s", extension, c.cfg.Server)
	if err := sip.ParseUri(parseURI, &recipient); err != nil {
		return err
	}
	req := sip.NewRequest(sip.SUBSCRIBE, recipient)
	req.AppendHeader(sip.NewHeader("Event", "dialog"))
	req.AppendHeader(sip.NewHeader("Expires", "3600"))
	req.AppendHeader(sip.NewHeader("Accept", "application/dialog-info+xml"))
	req.SetTransport(strings.ToUpper(c.cfg.Transport))

	tx, err := c.client.TransactionRequest(ctx, req, sipgo.ClientRequestBuild, sipgo.ClientRequestAddVia)
	if err != nil {
		return err
	}
	defer tx.Terminate()

	res, err := c.getResponse(tx)
	if err != nil {
		return err
	}

	if res.StatusCode == 401 {
		wwwAuth := res.GetHeader("WWW-Authenticate")
		if wwwAuth == nil {
			return fmt.Errorf("subscribe %s: 401 without WWW-Authenticate", extension)
		}
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
			return fmt.Errorf("subscribe %s: parse challenge: %w", extension, err)
		}
		cred, err := digest.Digest(chal, digest.Options{
			Method:   req.Method.String(),
			URI:      recipient.Host,
			Username: c.cfg.Username,
			Password: c.cfg.Password,
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: digest: %w", extension, err)
		}
		newReq := req.Clone()
		newReq.RemoveHeader("Via")
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))
		tx2, err := c.client.TransactionRequest(ctx, newReq, sipgo.ClientRequestIncreaseCSEQ, sipgo.ClientRequestAddVia)
		if err != nil {
			return err
		}
		defer tx2.Terminate()
		res, err = c.getResponse(tx2)
		if err != nil {
			return err
		}
	}

	if res.StatusCode != 200 && res.StatusCode != 202 {
		return fmt.Errorf("subscribe %s: %d", extension, res.StatusCode)
	}
	return nil
}

// contactAddr returns the Contact header value (sip:user@host or sip:user@host:port).
func (c *Client) contactAddr() string {
	if c.cfg.ContactPort > 0 && c.cfg.ContactPort != 5060 {
		return fmt.Sprintf("<sip:%s@%s:%d>", c.cfg.Username, c.cfg.ContactIP, c.cfg.ContactPort)
	}
	return fmt.Sprintf("<sip:%s@%s>", c.cfg.Username, c.cfg.ContactIP)
}

func (c *Client) getResponse(tx sip.ClientTransaction) (*sip.Response, error) {
	select {
	case <-tx.Done():
		return nil, fmt.Errorf("transaction died")
	case res := <-tx.Responses():
		return res, nil
	}
}

func (c *Client) handleNOTIFY(req *sip.Request, tx sip.ServerTransaction) {
	// Respond 200 OK immediately per RFC 3265
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if err := tx.Respond(res); err != nil {
		c.log.Error("NOTIFY 200 respond failed", "error", err)
		return
	}

	body := req.Body()
	if len(body) == 0 {
		return
	}

	extension := blf.ExtensionFromDialogInfo(body)
	if extension == "" {
		// Fallback: try To header (some PBXs send NOTIFY with To = monitored resource)
		if to := req.GetHeader("To"); to != nil {
			// Parse sip:user@host from To
			val := to.Value()
			if idx := strings.Index(val, ":"); idx >= 0 {
				rest := val[idx+1:]
				if end := strings.IndexAny(rest, ">;"); end >= 0 {
					rest = rest[:end]
				}
				if at := strings.Index(rest, "@"); at >= 0 {
					extension = rest[:at]
				}
			}
		}
	}

	state := blf.ParseDialogInfo(body)
	if state == blf.StateUnknown {
		state = blf.ParsePresenceBody(body)
	}

	if extension != "" && c.onBLF != nil {
		c.onBLF(extension, state)
	}
}
