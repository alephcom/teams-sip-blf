package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/darrenwiebe/teams_freepbx/internal/blf"
	"github.com/darrenwiebe/teams_freepbx/internal/cucm"
	"github.com/darrenwiebe/teams_freepbx/internal/graph"
	"github.com/darrenwiebe/teams_freepbx/internal/provider"
	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

func main() {
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load()

	extensionsPath := getEnv("EXTENSIONS_JSON", "config/extensions.json")
	voicemailConf := strings.TrimSpace(getEnv("VOICEMAIL_CONF", ""))
	statePath := getEnv("PRESENCE_STATE_JSON", "config/presence-state.json")
	providerName := strings.ToLower(strings.TrimSpace(getEnv("PROVIDER", "sip")))

	var extensions []ExtensionEntry
	var loadedFrom string
	if voicemailConf != "" {
		if _, err := os.Stat(voicemailConf); err != nil {
			slog.Error("voicemail conf file not found", "path", voicemailConf, "error", err)
			os.Exit(1)
		}
		var err error
		extensions, err = loadExtensionsVoicemail(voicemailConf)
		if err != nil {
			slog.Error("load voicemail conf", "error", err, "path", voicemailConf)
			os.Exit(1)
		}
		loadedFrom = voicemailConf
	} else {
		var err error
		extensions, loadedFrom, err = loadExtensionsFromPath(extensionsPath)
		if err != nil {
			slog.Error("load extensions", "error", err, "path", extensionsPath)
			os.Exit(1)
		}
	}
	slog.Info("loaded extensions", "count", len(extensions), "from", loadedFrom)

	extList := make([]string, 0, len(extensions))
	emailByExt := make(map[string]string)
	for _, e := range extensions {
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

	onLineState := func(extension string, state blf.State) {
		email, ok := emailByExt[extension]
		if !ok {
			slog.Warn("line state for unknown extension", "extension", extension)
			return
		}
		availability, activity := state.ToGraph()
		ctx := context.Background()
		if err := graphClient.SetPresence(ctx, email, extension, availability, activity); err != nil {
			slog.Error("set presence", "extension", extension, "email", email, "error", err)
			return
		}
		slog.Info("presence updated", "extension", extension, "state", state, "availability", availability)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var lineProvider provider.Provider
	switch providerName {
	case "sip":
		lineProvider, err = newSIPProvider(extList, onLineState)
	case "cucm":
		lineProvider = newCUCMProvider(onLineState)
	default:
		slog.Error("unknown PROVIDER (use sip or cucm)", "provider", providerName)
		os.Exit(1)
	}
	if err != nil {
		slog.Error("create provider", "provider", providerName, "error", err)
		os.Exit(1)
	}
	defer lineProvider.Close()

	if err := lineProvider.Start(ctx); err != nil {
		slog.Error("start provider", "provider", providerName, "error", err)
		os.Exit(1)
	}

	slog.Info("sip-blf-sync running", "provider", providerName, "extensions", len(extList))
	<-ctx.Done()
	slog.Info("shutting down")
}

func newSIPProvider(extList []string, onLineState provider.Handler) (provider.Provider, error) {
	stunServersRaw := strings.Split(getEnv("STUN_SERVERS", "stun.l.google.com,stun2.l.google.com,stun3.l.google.com,stun4.l.google.com"), ",")
	stunServers := make([]string, 0, len(stunServersRaw))
	for _, s := range stunServersRaw {
		if s := strings.TrimSpace(s); s != "" {
			stunServers = append(stunServers, s)
		}
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

	if err := sip.ResolveContactIfNeeded(&sipCfg, slog.Default()); err != nil {
		return nil, err
	}
	if sip.IsContactSentinel(sipCfg.ContactIP) {
		return nil, errContactUnresolved
	}

	listenAddr := strings.TrimSpace(getEnv("SIP_LISTEN", defaultListenAddr(sipCfg)))
	return provider.NewSIP(provider.SIPConfig{
		Config:     sipCfg,
		ListenAddr: listenAddr,
	}, extList, onLineState)
}

func newCUCMProvider(onLineState provider.Handler) provider.Provider {
	return cucm.NewServer(cucm.Config{
		ListenAddr: strings.TrimSpace(getEnv("CUCM_EVENT_LISTEN", "127.0.0.1:8090")),
		Token:      getEnv("CUCM_EVENT_TOKEN", ""),
	}, onLineState)
}

// errContactUnresolved is returned when STUN was requested but ContactIP is still a sentinel.
var errContactUnresolved = &contactUnresolvedError{}

type contactUnresolvedError struct{}

func (e *contactUnresolvedError) Error() string {
	return "SIP_CONTACT_IP is auto/stun/empty but STUN did not set a valid address; check STUN_SERVERS and network"
}
