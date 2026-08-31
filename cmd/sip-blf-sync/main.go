package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/darrenwiebe/teams_freepbx/internal/blf"
	"github.com/darrenwiebe/teams_freepbx/internal/extensions"
	"github.com/darrenwiebe/teams_freepbx/internal/graph"
	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

func main() {
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load()

	level := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	extensionsPath := getEnv("EXTENSIONS_JSON", "config/extensions.json")
	voicemailConf := strings.TrimSpace(getEnv("VOICEMAIL_CONF", ""))
	extensionsURL := strings.TrimSpace(getEnv("EXTENSIONS_URL", ""))
	extensionsToken := strings.TrimSpace(getEnv("EXTENSIONS_TOKEN", ""))
	refreshSec, _ := strconv.Atoi(getEnv("EXTENSIONS_REFRESH_SECONDS", "0"))
	statePath := getEnv("PRESENCE_STATE_JSON", "config/presence-state.json")
	statusMessage := strings.TrimSpace(getEnv("STATUS_MESSAGE", ""))
	statusTZ := strings.TrimSpace(getEnv("STATUS_MESSAGE_TIMEZONE", "UTC"))
	if statusTZ == "" {
		statusTZ = "UTC"
	}
	var statusExpiryDur *time.Duration
	if expiryRaw := strings.TrimSpace(getEnv("STATUS_MESSAGE_EXPIRY", "")); expiryRaw != "" {
		d, err := time.ParseDuration(expiryRaw)
		if err != nil {
			slog.Error("STATUS_MESSAGE_EXPIRY", "error", err, "value", expiryRaw)
			os.Exit(1)
		}
		statusExpiryDur = &d
	}

	var list []extensions.Entry
	var loadedFrom string
	switch {
	case voicemailConf != "":
		if _, err := os.Stat(voicemailConf); err != nil {
			slog.Error("voicemail conf file not found", "path", voicemailConf, "error", err)
			os.Exit(1)
		}
		var err error
		list, err = extensions.LoadVoicemail(voicemailConf)
		if err != nil {
			slog.Error("load voicemail conf", "error", err, "path", voicemailConf)
			os.Exit(1)
		}
		loadedFrom = voicemailConf
	case extensionsURL != "":
		var err error
		list, err = extensions.LoadFromURL(context.Background(), extensionsURL, extensionsToken)
		if err != nil {
			slog.Error("load extensions URL", "error", err, "url", extensionsURL)
			os.Exit(1)
		}
		loadedFrom = extensionsURL
	default:
		var err error
		list, loadedFrom, err = extensions.LoadFromPath(extensionsPath)
		if err != nil {
			slog.Error("load extensions", "error", err, "path", extensionsPath)
			os.Exit(1)
		}
	}
	slog.Info("loaded extensions", "count", len(list), "from", loadedFrom)

	extList := make([]string, 0, len(list))
	var emailMu sync.RWMutex
	emailByExt := make(map[string]string, len(list))
	for _, e := range list {
		extList = append(extList, e.Extension)
		emailByExt[e.Extension] = e.Email
	}

	graphClient, err := graph.NewClient(
		getEnv("AZURE_TENANT_ID", ""),
		getEnv("AZURE_CLIENT_ID", ""),
		getEnv("AZURE_CLIENT_SECRET", ""),
		statePath,
	)
	if err != nil {
		slog.Error("create graph client", "error", err)
		os.Exit(1)
	}

	onBLF := func(extension string, state blf.State) {
		emailMu.RLock()
		email, ok := emailByExt[extension]
		emailMu.RUnlock()
		if !ok {
			slog.Warn("BLF for unknown extension", "extension", extension)
			return
		}
		slog.Debug("BLF state", "extension", extension, "state", state)

		availability, activity := state.ToGraph()
		ctx := context.Background()
		if err := graphClient.SetPresence(ctx, email, extension, availability, activity); err != nil {
			slog.Error("set presence", "extension", extension, "email", email, "error", err)
			return
		}
		if statusMessage != "" {
			var expiry *time.Time
			if statusExpiryDur != nil {
				t := time.Now().Add(*statusExpiryDur)
				expiry = &t
			}
			if err := graphClient.SetStatusMessage(ctx, email, statusMessage, expiry, statusTZ); err != nil {
				slog.Error("set status message", "extension", extension, "email", email, "error", err)
			}
		}
		switch state {
		case blf.StateBusy, blf.StateIdle:
			slog.Info("presence updated", "extension", extension, "state", state, "availability", availability)
		}
	}

	stunServersRaw := strings.Split(getEnv("STUN_SERVERS", "stun.l.google.com,stun2.l.google.com,stun3.l.google.com,stun4.l.google.com"), ",")
	stunServers := make([]string, 0, len(stunServersRaw))
	for _, s := range stunServersRaw {
		if s := strings.TrimSpace(s); s != "" {
			stunServers = append(stunServers, s)
		}
	}
	contactPortEnv, err := parseContactPort(getEnv("SIP_CONTACT_PORT", ""))
	if err != nil {
		slog.Error("SIP_CONTACT_PORT", "error", err)
		os.Exit(1)
	}

	sipCfg := sip.Config{
		Server:      strings.TrimSpace(getEnv("SIP_SERVER", "127.0.0.1:5060")),
		Transport:   strings.TrimSpace(getEnv("SIP_TRANSPORT", "udp")),
		Username:    strings.TrimSpace(getEnv("SIP_USERNAME", "blf-client")),
		Password:    getEnv("SIP_PASSWORD", ""),
		ContactIP:   strings.TrimSpace(getEnv("SIP_CONTACT_IP", "127.0.0.1")),
		STUNServers: stunServers,
		UserAgent:   "teams-freepbx-blf/1.0",
	}
	useSTUN := sip.IsContactSentinel(sipCfg.ContactIP)

	if err := sip.ResolveContactIfNeeded(&sipCfg, slog.Default()); err != nil {
		slog.Error("STUN discovery failed", "error", err)
		os.Exit(1)
	}
	if sip.IsContactSentinel(sipCfg.ContactIP) {
		slog.Error("SIP_CONTACT_IP is auto/stun/empty but STUN did not set a valid address; check STUN_SERVERS and network")
		os.Exit(1)
	}

	if contactPortEnv > 0 {
		if useSTUN {
			slog.Warn("SIP_CONTACT_PORT ignored when using STUN; listen and Contact stay on 5060",
				"sip_contact_port", contactPortEnv)
		} else {
			sipCfg.ContactPort = contactPortEnv
		}
	}

	sipClient, err := sip.NewClient(sipCfg, extList, onBLF)
	if err != nil {
		slog.Error("create sip client", "error", err)
		os.Exit(1)
	}
	defer sipClient.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if extensionsURL != "" && refreshSec > 0 {
		extensions.StartRefresh(ctx, extensionsURL, extensionsToken, time.Duration(refreshSec)*time.Second,
			func(updated []extensions.Entry) {
				next := make(map[string]string, len(updated))
				for _, e := range updated {
					next[e.Extension] = e.Email
				}
				emailMu.Lock()
				emailByExt = next
				emailMu.Unlock()
			},
			func(format string, args ...any) {
				slog.Info(fmt.Sprintf(format, args...))
			},
		)
	}

	listenAddr := strings.TrimSpace(getEnv("SIP_LISTEN", defaultListenAddr(sipCfg, useSTUN)))
	if listenPort, err := listenPortFromAddr(listenAddr); err == nil && sipCfg.ContactPort > 0 && listenPort != sipCfg.ContactPort {
		slog.Warn("SIP_LISTEN port differs from ContactPort; PBX NOTIFYs use Contact",
			"listen", listenAddr, "contact_port", sipCfg.ContactPort)
	}

	go func() {
		if err := sipClient.ListenAndServe(ctx, sipCfg.Transport, listenAddr); err != nil && ctx.Err() == nil {
			slog.Error("sip server", "error", err)
		}
	}()

	regExpires, err := sipClient.Register(ctx)
	if err != nil {
		slog.Error("register", "error", err)
		os.Exit(1)
	}

	subExpires, err := sipClient.Subscribe(ctx)
	if err != nil {
		slog.Error("subscribe", "error", err)
		os.Exit(1)
	}

	slog.Info("sip-blf-sync running",
		"extensions", len(extList),
		"contact_ip", sipCfg.ContactIP,
		"contact_port", contactListenPort(sipCfg),
		"listen", listenAddr,
		"register_expires", regExpires,
		"subscribe_expires", subExpires,
	)

	go maintainSIPSession(ctx, sipClient, regExpires, subExpires)

	<-ctx.Done()
	slog.Info("shutting down")
}

const sipRefreshFailLimit = 3

// maintainSIPSession re-REGISTERs and re-SUBSCRIBEs before granted Expires.
// After sipRefreshFailLimit consecutive failures of either path, exits so supervisord can restart.
func maintainSIPSession(ctx context.Context, client *sip.Client, regExpires, subExpires time.Duration) {
	regTimer := time.NewTimer(sip.RefreshDelay(regExpires))
	subTimer := time.NewTimer(sip.RefreshDelay(subExpires))
	defer regTimer.Stop()
	defer subTimer.Stop()

	regFails, subFails := 0, 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-regTimer.C:
			expires, err := client.Register(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				regFails++
				slog.Error("re-register failed", "error", err, "consecutive_failures", regFails)
				if regFails >= sipRefreshFailLimit {
					slog.Error("re-register failed too many times; exiting for restart", "limit", sipRefreshFailLimit)
					os.Exit(1)
				}
				regTimer.Reset(time.Minute)
				continue
			}
			regFails = 0
			regExpires = expires
			delay := sip.RefreshDelay(regExpires)
			slog.Info("re-registered", "expires", regExpires, "next_refresh", delay)
			regTimer.Reset(delay)
		case <-subTimer.C:
			expires, err := client.Subscribe(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				subFails++
				slog.Error("re-subscribe failed", "error", err, "consecutive_failures", subFails)
				if subFails >= sipRefreshFailLimit {
					slog.Error("re-subscribe failed too many times; exiting for restart", "limit", sipRefreshFailLimit)
					os.Exit(1)
				}
				subTimer.Reset(time.Minute)
				continue
			}
			subFails = 0
			subExpires = expires
			delay := sip.RefreshDelay(subExpires)
			slog.Info("re-subscribed", "expires", subExpires, "next_refresh", delay)
			subTimer.Reset(delay)
		}
	}
}
