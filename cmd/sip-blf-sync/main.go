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
	"github.com/darrenwiebe/teams_freepbx/internal/graph"
	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

func main() {
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load()

	extensionsPath := getEnv("EXTENSIONS_JSON", "config/extensions.json")
	statePath := getEnv("PRESENCE_STATE_JSON", "config/presence-state.json")

	extensions, err := loadExtensions(extensionsPath)
	if err != nil {
		slog.Error("load extensions", "error", err, "path", extensionsPath)
		os.Exit(1)
	}

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

	onBLF := func(extension string, state blf.State) {
		email, ok := emailByExt[extension]
		if !ok {
			slog.Warn("BLF for unknown extension", "extension", extension)
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
		slog.Error("STUN discovery failed", "error", err)
		os.Exit(1)
	}
	if sip.IsContactSentinel(sipCfg.ContactIP) {
		slog.Error("SIP_CONTACT_IP is auto/stun/empty but STUN did not set a valid address; check STUN_SERVERS and network")
		os.Exit(1)
	}

	sipClient, err := sip.NewClient(sipCfg, extList, onBLF)
	if err != nil {
		slog.Error("create sip client", "error", err)
		os.Exit(1)
	}
	defer sipClient.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		listenAddr := strings.TrimSpace(getEnv("SIP_LISTEN", defaultListenAddr(sipCfg)))
		if err := sipClient.ListenAndServe(ctx, sipCfg.Transport, listenAddr); err != nil && ctx.Err() == nil {
			slog.Error("sip server", "error", err)
		}
	}()

	if err := sipClient.Register(ctx); err != nil {
		slog.Error("register", "error", err)
		os.Exit(1)
	}

	if err := sipClient.Subscribe(ctx); err != nil {
		slog.Error("subscribe", "error", err)
		os.Exit(1)
	}

	slog.Info("sip-blf-sync running", "extensions", len(extList))
	<-ctx.Done()
	slog.Info("shutting down")
}
