package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// PullPointSubscription represents an active PullPoint subscription session
type PullPointSubscription struct {
	SubscriptionURL string
	CurrentTime     string
	TerminationTime string
}

// PullMessagesResult wraps the notifications and diagnostic data
type PullMessagesResult struct {
	CurrentTime     string
	TerminationTime string
	Messages        []NotificationMessage
	RawResponse     string
	RawRequest      string
	Duration        time.Duration
}

// CreatePullPointSubscription requests a new PullPoint subscription from the Events service
func (c *Client) CreatePullPointSubscription(ctx context.Context, initialTermination string) (*PullPointSubscription, error) {
	eventsXAddr := c.GetCachedEventsXAddr()
	if eventsXAddr == "" {
		return nil, fmt.Errorf("events service XAddr is not resolved")
	}

	if initialTermination == "" {
		initialTermination = "PT60S"
	}

	body := fmt.Sprintf(`<tev:CreatePullPointSubscription xmlns:tev="%s">
    <tev:InitialTerminationTime>%s</tev:InitialTerminationTime>
  </tev:CreatePullPointSubscription>`, NSTEvents, xmlEscape(initialTermination))

	action := "http://www.onvif.org/ver10/events/wsdl/EventPortType/CreatePullPointSubscriptionRequest"

	res, err := c.SendSOAP(ctx, eventsXAddr, action, body, true)
	if err != nil {
		return nil, fmt.Errorf("CreatePullPointSubscription failed: %w", err)
	}

	var env CreatePullPointSubscriptionEnvelope
	if err := xml.Unmarshal(res.ResponseBody, &env); err != nil {
		return nil, fmt.Errorf("failed to parse CreatePullPointSubscription response: %w", err)
	}
	resp := env.Body.Response

	subURL := c.SanitizeURL(resp.SubscriptionReference.Address)
	if subURL == "" {
		return nil, fmt.Errorf("CreatePullPointSubscription returned empty subscription address")
	}

	return &PullPointSubscription{
		SubscriptionURL: subURL,
		CurrentTime:     resp.CurrentTime,
		TerminationTime: resp.TerminationTime,
	}, nil
}

// PullMessages polls the subscription endpoint for queued notification messages
func (c *Client) PullMessages(ctx context.Context, subscriptionURL string, timeout string, messageLimit int) (*PullMessagesResult, error) {
	if subscriptionURL == "" {
		return nil, fmt.Errorf("subscription URL is required")
	}
	if timeout == "" {
		timeout = "PT10S"
	}
	if messageLimit <= 0 {
		messageLimit = 100
	}

	body := fmt.Sprintf(`<tev:PullMessages xmlns:tev="%s">
    <tev:Timeout>%s</tev:Timeout>
    <tev:MessageLimit>%d</tev:MessageLimit>
  </tev:PullMessages>`, NSTEvents, xmlEscape(timeout), messageLimit)

	action := "http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/PullMessagesRequest"

	res, err := c.SendSOAP(ctx, subscriptionURL, action, body, true)
	if err != nil {
		return nil, fmt.Errorf("PullMessages failed: %w", err)
	}

	var env PullMessagesEnvelope
	if err := xml.Unmarshal(res.ResponseBody, &env); err != nil {
		return nil, fmt.Errorf("failed to parse PullMessages response: %w", err)
	}
	resp := env.Body.Response

	return &PullMessagesResult{
		CurrentTime:     resp.CurrentTime,
		TerminationTime: resp.TerminationTime,
		Messages:        resp.NotificationMessages,
		RawResponse:     res.RawResponse,
		RawRequest:      res.RawRequest,
		Duration:        res.Duration,
	}, nil
}

// Renew extends the lifetime of an active PullPoint subscription
func (c *Client) Renew(ctx context.Context, subscriptionURL string, terminationTime string) error {
	if subscriptionURL == "" {
		return fmt.Errorf("subscription URL is required")
	}
	if terminationTime == "" {
		terminationTime = "PT60S"
	}

	body := fmt.Sprintf(`<wsnt:Renew xmlns:wsnt="%s">
    <wsnt:TerminationTime>%s</wsnt:TerminationTime>
  </wsnt:Renew>`, NSWSNT, xmlEscape(terminationTime))

	action := "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/RenewRequest"

	_, err := c.SendSOAP(ctx, subscriptionURL, action, body, true)
	if err != nil {
		return fmt.Errorf("Renew failed: %w", err)
	}
	return nil
}

// Unsubscribe immediately terminates the PullPoint subscription on the camera
func (c *Client) Unsubscribe(ctx context.Context, subscriptionURL string) error {
	if subscriptionURL == "" {
		return nil
	}

	body := fmt.Sprintf(`<wsnt:Unsubscribe xmlns:wsnt="%s"/>`, NSWSNT)
	action := "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest"

	_, err := c.SendSOAP(ctx, subscriptionURL, action, body, true)
	if err != nil {
		return fmt.Errorf("Unsubscribe failed: %w", err)
	}
	return nil
}

// Subscribe requests a new WS-BaseNotification push subscription from the Events service
func (c *Client) Subscribe(ctx context.Context, consumerURL string, initialTermination string) (*PushSubscription, error) {
	eventsXAddr := c.GetCachedEventsXAddr()
	if eventsXAddr == "" {
		return nil, fmt.Errorf("events service XAddr is not resolved")
	}

	if initialTermination == "" {
		initialTermination = "PT60S"
	}

	body := fmt.Sprintf(`<wsnt:Subscribe xmlns:wsnt="%s" xmlns:wsa="%s">
    <wsnt:ConsumerReference>
      <wsa:Address>%s</wsa:Address>
    </wsnt:ConsumerReference>
    <wsnt:InitialTerminationTime>%s</wsnt:InitialTerminationTime>
  </wsnt:Subscribe>`, NSWSNT, NSWSAddressing, xmlEscape(consumerURL), xmlEscape(initialTermination))

	action := "http://docs.oasis-open.org/wsn/bw-2/NotificationProducer/SubscribeRequest"

	res, err := c.SendSOAP(ctx, eventsXAddr, action, body, true)
	if err != nil {
		return nil, fmt.Errorf("Subscribe failed: %w", err)
	}

	var env SubscribeEnvelope
	if err := xml.Unmarshal(res.ResponseBody, &env); err != nil {
		return nil, fmt.Errorf("failed to parse Subscribe response: %w", err)
	}
	resp := env.Body.Response

	subURL := c.SanitizeURL(resp.SubscriptionReference.Address)
	if subURL == "" {
		return nil, fmt.Errorf("Subscribe returned empty subscription address")
	}

	return &PushSubscription{
		SubscriptionURL: subURL,
		CurrentTime:     resp.CurrentTime,
		TerminationTime: resp.TerminationTime,
	}, nil
}

// SubscriptionState describes the health of an ONVIF event stream
type SubscriptionState string

const (
	StateActive       SubscriptionState = "Active"
	StateReconnecting SubscriptionState = "Reconnecting"
	StateFailed       SubscriptionState = "Failed"
	StateClosed       SubscriptionState = "Closed"
)

// EventHandler is called when messages or status changes occur
type EventHandler interface {
	OnStatusChange(state SubscriptionState, reason string)
	OnNotification(msg NotificationMessage, rawRequest string, rawResponse string)
}

// SubscriptionMode defines whether the client pulls messages or camera pushes via webhook
type SubscriptionMode string

const (
	ModePullPoint SubscriptionMode = "PullPoint"
	ModePush      SubscriptionMode = "Push"
)

// SubscriptionManager manages long-polling, renewing, and graceful cleanup of an ONVIF camera
type SubscriptionManager struct {
	client       *Client
	handler      EventHandler
	sub          *PullPointSubscription
	pushSub      *PushSubscription
	state        SubscriptionState
	stateMu      sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
	renewTicker  *time.Ticker
	pullTimeout  string
	subTTL       string
	renewPeriod  time.Duration
	pushReceiver *PushReceiver
	camKey       string
	targetIP     string
	mode         SubscriptionMode
	pullFailures int
}

// NewSubscriptionManager initializes a manager for a camera client
func NewSubscriptionManager(client *Client, handler EventHandler) *SubscriptionManager {
	return &SubscriptionManager{
		client:      client,
		handler:     handler,
		state:       StateReconnecting,
		stopCh:      make(chan struct{}),
		pullTimeout: "PT10S",
		subTTL:      "PT60S",
		renewPeriod: 25 * time.Second,
		mode:        ModePullPoint,
	}
}

// EnablePushFallback equips the manager with a PushReceiver for automatic webhook fallback
func (sm *SubscriptionManager) EnablePushFallback(pr *PushReceiver, camKey string, targetIP string) {
	sm.stateMu.Lock()
	defer sm.stateMu.Unlock()
	sm.pushReceiver = pr
	sm.camKey = camKey
	sm.targetIP = targetIP
}

// GetMode returns current subscription mode (PullPoint or Push)
func (sm *SubscriptionManager) GetMode() SubscriptionMode {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.mode
}

// GetState returns current subscription state
func (sm *SubscriptionManager) GetState() SubscriptionState {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.state
}

func (sm *SubscriptionManager) setState(s SubscriptionState, reason string) {
	sm.stateMu.Lock()
	sm.state = s
	sm.stateMu.Unlock()
	if sm.handler != nil {
		sm.handler.OnStatusChange(s, reason)
	}
}

// Start launches the subscription polling and renewal workers in background
func (sm *SubscriptionManager) Start(ctx context.Context) {
	sm.wg.Add(1)
	go sm.run(ctx)
}

// Stop cleanly unsubscribes and cancels workers
func (sm *SubscriptionManager) Stop() {
	select {
	case <-sm.stopCh:
		return // already stopped
	default:
		close(sm.stopCh)
	}

	sm.wg.Wait()

	// Drain / unsubscribe on shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if sm.pushSub != nil && sm.pushSub.SubscriptionURL != "" {
		_ = sm.client.Unsubscribe(shutdownCtx, sm.pushSub.SubscriptionURL)
		if sm.pushReceiver != nil {
			sm.pushReceiver.UnregisterHandler(sm.camKey)
		}
		sm.pushSub = nil
	}

	if sm.sub != nil && sm.sub.SubscriptionURL != "" {
		_ = sm.client.Unsubscribe(shutdownCtx, sm.sub.SubscriptionURL)
		sm.sub = nil
	}

	sm.setState(StateClosed, "Stopped by user")
}

func (sm *SubscriptionManager) run(parentCtx context.Context) {
	defer sm.wg.Done()

	// Exponential backoff configuration
	baseBackoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	currentBackoff := baseBackoff

	for {
		select {
		case <-parentCtx.Done():
			return
		case <-sm.stopCh:
			return
		default:
		}

		if sm.mode == ModePush {
			sm.runPushLoop(parentCtx, &currentBackoff, baseBackoff, maxBackoff)
			continue
		}

		// Ensure we have a valid PullPoint subscription
		if sm.sub == nil {
			sm.setState(StateReconnecting, "Establishing PullPoint subscription...")
			subCtx, cancelSub := context.WithTimeout(parentCtx, 15*time.Second)
			sub, err := sm.client.CreatePullPointSubscription(subCtx, sm.subTTL)
			cancelSub()

			if err != nil {
				// If CreatePullPoint fails and push fallback is available, switch to push
				if sm.pushReceiver != nil {
					sm.mode = ModePush
					sm.setState(StateReconnecting, fmt.Sprintf("PullPoint unavailable (%v); switching to Push webhook...", err))
					continue
				}
				sm.setState(StateFailed, fmt.Sprintf("CreatePullPoint failed: %v", err))
				currentBackoff = applyJitterSleep(parentCtx, sm.stopCh, currentBackoff, maxBackoff)
				continue
			}

			sm.sub = sub
			sm.setState(StateActive, "Subscription active (PullPoint)")
			currentBackoff = baseBackoff // reset backoff upon success
		}

		// Renew tick check
		renewCtx, cancelRenew := context.WithTimeout(parentCtx, 5*time.Second)
		_ = sm.client.Renew(renewCtx, sm.sub.SubscriptionURL, sm.subTTL)
		cancelRenew()

		// Execute PullMessages
		pullCtx, cancelPull := context.WithTimeout(parentCtx, 20*time.Second)
		res, err := sm.client.PullMessages(pullCtx, sm.sub.SubscriptionURL, sm.pullTimeout, 100)
		cancelPull()

		if err != nil {
			sm.pullFailures++
			isEOF := strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "connection reset")
			if sm.pushReceiver != nil && (isEOF || sm.pullFailures >= 2) {
				sm.setState(StateReconnecting, "PullMessages rejected by camera (EOF); switching to Push webhook...")
				// Clean up broken pull subscription
				if sm.sub != nil {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = sm.client.Unsubscribe(shutdownCtx, sm.sub.SubscriptionURL)
					cancel()
					sm.sub = nil
				}
				sm.mode = ModePush
				continue
			}

			sm.setState(StateReconnecting, fmt.Sprintf("PullMessages error: %v", err))
			// Subscription may have expired or been rejected; clear it so we re-subscribe
			sm.sub = nil
			currentBackoff = applyJitterSleep(parentCtx, sm.stopCh, currentBackoff, maxBackoff)
			continue
		}

		sm.pullFailures = 0
		// Reset backoff on successful pull
		currentBackoff = baseBackoff
		sm.setState(StateActive, "Subscription active (PullPoint)")

		if res != nil && len(res.Messages) > 0 {
			for _, msg := range res.Messages {
				if sm.handler != nil {
					sm.handler.OnNotification(msg, res.RawRequest, res.RawResponse)
				}
			}
		}
	}
}

func (sm *SubscriptionManager) runPushLoop(parentCtx context.Context, currentBackoff *time.Duration, baseBackoff, maxBackoff time.Duration) {
	if sm.pushReceiver == nil {
		sm.mode = ModePullPoint
		return
	}

	if sm.pushSub == nil {
		sm.setState(StateReconnecting, "Establishing Push webhook subscription...")
		consumerURL, err := sm.pushReceiver.GetConsumerURL(sm.targetIP, sm.camKey)
		if err != nil {
			sm.setState(StateFailed, fmt.Sprintf("Failed resolving consumer webhook URL: %v", err))
			*currentBackoff = applyJitterSleep(parentCtx, sm.stopCh, *currentBackoff, maxBackoff)
			return
		}

		sm.pushReceiver.RegisterHandler(sm.camKey, sm.handler)

		subCtx, cancelSub := context.WithTimeout(parentCtx, 15*time.Second)
		pushSub, err := sm.client.Subscribe(subCtx, consumerURL, sm.subTTL)
		cancelSub()

		if err != nil {
			sm.setState(StateFailed, fmt.Sprintf("Push Subscribe failed: %v", err))
			*currentBackoff = applyJitterSleep(parentCtx, sm.stopCh, *currentBackoff, maxBackoff)
			return
		}

		sm.pushSub = pushSub
		*currentBackoff = baseBackoff
		sm.setState(StateActive, "Subscription active (Push webhook)")
	}

	// Sleep until renew period or shutdown signal
	select {
	case <-parentCtx.Done():
		return
	case <-sm.stopCh:
		return
	case <-time.After(sm.renewPeriod):
	}

	// Renew push subscription
	renewCtx, cancelRenew := context.WithTimeout(parentCtx, 5*time.Second)
	err := sm.client.Renew(renewCtx, sm.pushSub.SubscriptionURL, sm.subTTL)
	cancelRenew()

	if err != nil {
		sm.setState(StateReconnecting, fmt.Sprintf("Push renew error: %v", err))
		sm.pushSub = nil // force re-subscribe
		*currentBackoff = applyJitterSleep(parentCtx, sm.stopCh, *currentBackoff, maxBackoff)
		return
	}

	sm.setState(StateActive, "Subscription active (Push webhook)")
}

// applyJitterSleep sleeps with exponential backoff plus ±20% random jitter
func applyJitterSleep(ctx context.Context, stopCh <-chan struct{}, current time.Duration, max time.Duration) time.Duration {
	jitter := float64(current) * (0.8 + 0.4*rand.Float64())
	sleepDur := time.Duration(jitter)

	select {
	case <-time.After(sleepDur):
	case <-ctx.Done():
	case <-stopCh:
	}

	next := current * 2
	if next > max {
		next = max
	}
	return next
}
