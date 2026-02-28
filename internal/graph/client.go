package graph

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

const (
	graphScope = "https://graph.microsoft.com/.default"
	expiration = "PT1H" // 1 hour; valid range PT5M to PT4H
)

// Client sets Teams presence via Microsoft Graph (app-only auth).
type Client struct {
	graph       *msgraphsdk.GraphServiceClient
	clientID    string // application ID; required as sessionId for app-only SetPresence
	state       *SessionState
	log         *slog.Logger
	userIDCache map[string]string // UPN/email -> object ID (GUID); guarded by userIDCacheMu
	userIDCacheMu sync.RWMutex
}

// NewClient creates a Graph client using client credentials (tenant, client ID, secret)
// and the given session state for persistence of session IDs.
func NewClient(tenantID, clientID, clientSecret, statePath string) (*Client, error) {
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, err
	}
	graph, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, []string{graphScope})
	if err != nil {
		return nil, err
	}
	state, err := LoadSessionState(statePath)
	if err != nil {
		return nil, err
	}
	return &Client{
		graph:       graph,
		clientID:    clientID,
		state:       state,
		log:         slog.Default().With("component", "graph"),
		userIDCache: make(map[string]string),
	}, nil
}

// resolveUserID returns the Graph user object ID (GUID) for the given UPN or email.
// It caches results so each user is looked up only once.
func (c *Client) resolveUserID(ctx context.Context, upn string) (string, error) {
	c.userIDCacheMu.RLock()
	if id, ok := c.userIDCache[upn]; ok {
		c.userIDCacheMu.RUnlock()
		return id, nil
	}
	c.userIDCacheMu.RUnlock()

	user, err := c.graph.Users().ByUserId(upn).Get(ctx, nil)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", errors.New("user not found")
	}
	id := user.GetId()
	if id == nil || *id == "" {
		return "", errors.New("user has no id")
	}

	c.userIDCacheMu.Lock()
	c.userIDCache[upn] = *id
	c.userIDCacheMu.Unlock()
	c.log.Debug("resolved user to object ID", "upn", upn, "objectId", *id)
	return *id, nil
}

// SetPresence sets the user's Teams presence. userID is the user's email (userPrincipalName).
// The UPN is resolved to the Graph object ID (GUID) via GET /users/{upn}; the GUID is used for the presence call.
// availability and activity are Graph values (e.g. "Available", "Busy", "InACall").
// For app-only auth, sessionId must be the application (client) ID.
func (c *Client) SetPresence(ctx context.Context, userID, extension, availability, activity string) error {
	objectID, err := c.resolveUserID(ctx, userID)
	if err != nil {
		c.log.Error("resolve user ID failed", "user", userID, "extension", extension, "error", err)
		return err
	}

	body := users.NewItemPresenceSetPresencePostRequestBody()
	body.SetSessionId(&c.clientID)
	body.SetAvailability(&availability)
	body.SetActivity(&activity)
	dur, err := parseISODuration(expiration)
	if err != nil {
		return err
	}
	body.SetExpirationDuration(dur)

	reqConfig := &users.ItemPresenceSetPresenceRequestBuilderPostRequestConfiguration{}
	err = c.graph.Users().ByUserId(objectID).Presence().SetPresence().Post(ctx, body, reqConfig)
	if err != nil {
		c.log.Error("setPresence failed",
			"user", userID,
			"extension", extension,
			"availability", availability,
			"activity", activity,
			"error", err,
			"error_chain", errorChain(err))
		return err
	}
	c.log.Debug("setPresence ok", "user", userID, "extension", extension, "availability", availability)
	return nil
}

// errorChain returns a string of all errors in the chain for debugging.
func errorChain(err error) string {
	var s string
	for err != nil {
		if s != "" {
			s += "; "
		}
		s += err.Error()
		err = errors.Unwrap(err)
	}
	return s
}

func parseISODuration(s string) (*serialization.ISODuration, error) {
	return serialization.ParseISODuration(s)
}

// SetStatusMessage sets the user's presence status message (optional).
func (c *Client) SetStatusMessage(ctx context.Context, userID, message string) error {
	msg := models.NewPresenceStatusMessage()
	itemBody := models.NewItemBody()
	content := message
	contentType := models.TEXT_BODYTYPE
	itemBody.SetContent(&content)
	itemBody.SetContentType(&contentType)
	msg.SetMessage(itemBody)
	body := users.NewItemPresenceSetStatusMessagePostRequestBody()
	body.SetStatusMessage(msg)

	reqConfig := &users.ItemPresenceSetStatusMessageRequestBuilderPostRequestConfiguration{}
	err := c.graph.Users().ByUserId(userID).Presence().SetStatusMessage().Post(ctx, body, reqConfig)
	if err != nil {
		c.log.Error("setStatusMessage failed", "user", userID, "error", err)
		return err
	}
	return nil
}
