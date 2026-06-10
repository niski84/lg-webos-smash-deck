package lgdeck

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

// HTTPServer wires HTTP routes to LG TV commands.
type HTTPServer struct {
	mu           sync.RWMutex
	cfg          AppConfig
	settingsPath string
	logger       *Logger

	// volumeMu guards the cancellable context for in-flight volume changes.
	// A new POST /api/volume cancels the previous one before starting.
	volumeMu     sync.Mutex
	volumeCancel context.CancelFunc
}

func NewHTTPServer(cfg AppConfig) *HTTPServer {
	settingsPath := DefaultSettingsPath()
	logPath := filepath.Join(DataDir(), "lgdeck.log")
	logger := NewLogger(logPath)
	noop := func() {}
	s := &HTTPServer{
		cfg:          cfg,
		settingsPath: settingsPath,
		logger:       logger,
		volumeCancel: noop,
	}
	log.Printf("[lgdeck] settings file : %s", settingsPath)
	log.Printf("[lgdeck] activity log  : %s", logPath)
	return s
}

func (s *HTTPServer) getCfg() AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *HTTPServer) setCfg(c AppConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = c
}

// tvClient creates a TVClient from the current config. Returns nil if not configured.
func (s *HTTPServer) tvClient() (*TVClient, error) {
	cfg := s.getCfg()
	if cfg.TVIP == "" {
		return nil, fmt.Errorf("TV IP not configured")
	}
	return NewTVClient(cfg.TVIP, cfg.Keycode)
}

// ── JSON helpers ─────────────────────────────────────────────────────────────

type apiResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiResp{Success: true, Data: data})
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResp{Success: false, Error: msg})
}

func methodOnly(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}

// Routes registers all API endpoints and the static file server.
func (s *HTTPServer) Routes(webFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/power", s.handlePower)
	mux.HandleFunc("/api/volume", s.handleVolume)
	mux.HandleFunc("/api/volume/stream", s.handleVolumeStream)
	mux.HandleFunc("/api/mute", s.handleMute)
	mux.HandleFunc("/api/key", s.handleKey)
	mux.HandleFunc("/api/input", s.handleInput)
	mux.HandleFunc("/api/app", s.handleApp)
	mux.HandleFunc("/api/picture", s.handlePicture)
	mux.HandleFunc("/api/energy", s.handleEnergy)
	mux.HandleFunc("/api/screenmute", s.handleScreenMute)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/macaddress", s.handleMacAddress)

	mux.Handle("/", http.FileServer(http.FS(webFS)))
	return mux
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// handleHealth reports service status and whether a TV is configured.
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	cfg := s.getCfg()
	ok(w, map[string]any{
		"service": "lg-webos-smash-deck",
		"tv_ip":   cfg.TVIP,
		"configured": cfg.TVIP != "",
	})
}

// handleSettings reads, updates, or tests the TV connection settings.
func (s *HTTPServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.getCfg()
		maxVol := cfg.MaxVolume
		if maxVol <= 0 {
			maxVol = 100
		}
		ok(w, map[string]interface{}{
			"port":       cfg.Port,
			"tv_ip":      cfg.TVIP,
			"tv_mac":     cfg.TVMac,
			"keycode":    maskSecret(cfg.Keycode),
			"max_volume": maxVol,
		})

	case http.MethodPost:
		var body struct {
			Port      string `json:"port"`
			TVIP      string `json:"tv_ip"`
			TVMac     string `json:"tv_mac"`
			Keycode   string `json:"keycode"`
			MaxVolume int    `json:"max_volume"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		cur := s.getCfg()
		if body.Port != "" {
			cur.Port = body.Port
		}
		if body.TVIP != "" {
			cur.TVIP = body.TVIP
		}
		if body.TVMac != "" {
			cur.TVMac = body.TVMac
		}
		if body.Keycode != "" && !isMasked(body.Keycode) {
			cur.Keycode = body.Keycode
		}
		if body.MaxVolume > 0 && body.MaxVolume <= 100 {
			cur.MaxVolume = body.MaxVolume
		}
		if err := SaveAppConfig(s.settingsPath, cur); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.setCfg(cur)
		s.logger.Info("settings updated tv_ip=%s max_vol=%d", cur.TVIP, cur.MaxVolume)
		ok(w, map[string]bool{"saved": true})

	case http.MethodPut:
		// Test connection
		c, err := s.tvClient()
		if err != nil {
			ok(w, map[string]string{"status": err.Error()})
			return
		}
		if c.IsReachable() {
			s.logger.Info("connection test: TV reachable at %s", s.getCfg().TVIP)
			ok(w, map[string]string{"status": "reachable"})
		} else {
			ok(w, map[string]string{"status": "unreachable"})
		}

	default:
		fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleState returns the TV's current power, volume, and input state.
func (s *HTTPServer) handleState(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	c, err := s.tvClient()
	if err != nil {
		ok(w, TVState{Reachable: false, Error: err.Error()})
		return
	}
	if !c.IsReachable() {
		ok(w, TVState{Reachable: false, Error: "TV unreachable"})
		return
	}
	state := GetTVState(c)
	ok(w, state)
}

// handlePower turns the TV on via Wake-on-LAN or powers it off.
func (s *HTTPServer) handlePower(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cfg := s.getCfg()
	switch body.State {
	case "on":
		if cfg.TVMac == "" {
			fail(w, http.StatusBadRequest, "TV_MAC not configured (required for Wake-on-LAN)")
			return
		}
		if err := WakeOnLan(cfg.TVMac); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.logger.Info("WoL packet sent to %s", cfg.TVMac)
		ok(w, map[string]string{"status": "wol_sent"})

	case "off":
		c, err := s.tvClient()
		if err != nil {
			fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := PowerOff(c); err != nil {
			s.logger.Warn("power off failed: %v", err)
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		s.logger.Info("TV powered off")
		ok(w, map[string]string{"status": "off"})

	default:
		fail(w, http.StatusBadRequest, "state must be 'on' or 'off'")
	}
}

// handleVolume gets the current volume or sets it to a target level.
func (s *HTTPServer) handleVolume(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c, err := s.tvClient()
		if err != nil {
			fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		vol, err := GetCurrentVolume(c)
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		ok(w, map[string]int{"level": vol})

	case http.MethodPost:
		var body struct {
			Level int `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		c, err := s.tvClient()
		if err != nil {
			fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		// Clamp to configured max volume.
		if maxVol := s.getCfg().MaxVolume; maxVol > 0 && body.Level > maxVol {
			body.Level = maxVol
		}

		// Cancel any in-flight volume change before starting a new one.
		s.volumeMu.Lock()
		s.volumeCancel()
		ctx, cancel := context.WithCancel(context.Background())
		s.volumeCancel = cancel
		s.volumeMu.Unlock()

		// Use KEY_ACTION-based volume change so that HDMI CEC/SIMPLINK
		// (soundbars, AV receivers) respond AND the TV shows its OSD.
		err = SetVolumeByKey(ctx, c, body.Level)
		cancel() // release context resources
		if err != nil {
			if ctx.Err() != nil {
				// Cancelled by a newer request — silently discard.
				return
			}
			s.logger.Warn("set volume failed: %v", err)
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		// Return the confirmed TV volume so the UI slider snaps to the
		// actual level rather than the requested level.
		confirmed, err := GetCurrentVolume(c)
		if err != nil {
			confirmed = body.Level
		}
		s.logger.Info("volume set to %d (confirmed: %d)", body.Level, confirmed)
		ok(w, map[string]int{"level": confirmed})

	default:
		fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleVolumeStream adjusts volume one step at a time and streams each
// confirmed level back as a Server-Sent Event so the browser slider updates
// in real time. The stream ends when the target is reached or the client
// disconnects (slider moved again / page closed).
//
//   POST /api/volume/stream   body: {"level": N}
//   Response: text/event-stream
//   Events:   data: {"level":N,"done":false}   (each step)
//             data: {"level":N,"done":true}    (target reached)
func (s *HTTPServer) handleVolumeStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		Level int `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Level < 0 || body.Level > 100 {
		fail(w, http.StatusBadRequest, "level must be 0–100")
		return
	}
	// Honour the configured max volume ceiling (e.g. prevents blowing a
	// soundbar that clips at TV-level 70).
	if maxVol := s.getCfg().MaxVolume; maxVol > 0 && body.Level > maxVol {
		body.Level = maxVol
	}

	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// SSE headers — disable all buffering so events arrive immediately.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	emit := func(level int, done bool) {
		fmt.Fprintf(w, "data: {\"level\":%d,\"done\":%t}\n\n", level, done)
		flusher.Flush()
	}

	// Cancel any in-flight volume op (from a previous slider position).
	s.volumeMu.Lock()
	s.volumeCancel()
	ctx, cancel := context.WithCancel(r.Context())
	s.volumeCancel = cancel
	s.volumeMu.Unlock()
	defer cancel()

	const (
		maxSteps   = 101 // 0–100 range max
		verifyEvery = 3  // re-read actual TV volume every N presses to correct drift
		pressDelay = 50 * time.Millisecond // small pause between presses
	)

	// Use a fast read-idle timeout for KEY_ACTION on LAN: the 32-byte
	// encrypted response arrives in one TCP segment so 80ms is plenty.
	// CURRENT_VOL uses the default 500ms timeout for reliability.
	fastClient := *c // shallow copy
	fastClient.keyIdleTimeout = readIdleTimeoutFast

	// Bootstrap: read the actual current volume once.
	current, err := GetCurrentVolume(c)
	if err != nil {
		return
	}
	emit(current, false)

	for step := 0; step < maxSteps; step++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		delta := body.Level - current
		if delta == 0 {
			emit(current, true)
			s.logger.Info("volume reached %d", current)
			return
		}

		key := "volumeup"
		if delta < 0 {
			key = "volumedown"
		}

		// Send a single key press with fast timeout.
		SendKey(&fastClient, key) //nolint:errcheck

		// Optimistically track the predicted level for UI smoothness.
		if key == "volumeup" {
			current++
		} else {
			current--
		}
		emit(current, false)

		// Every verifyEvery presses, re-read the actual TV volume to catch
		// any dropped commands and correct course.
		if (step+1)%verifyEvery == 0 {
			if actual, err := GetCurrentVolume(c); err == nil {
				current = actual
				emit(current, false)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pressDelay):
		}
	}

	// Fell through maxSteps — emit confirmed final level.
	if final, err := GetCurrentVolume(c); err == nil {
		emit(final, true)
	}
}

// handleMute mutes or unmutes the TV audio.
func (s *HTTPServer) handleMute(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Muted bool `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := SetMute(c, body.Muted); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("mute set to %v", body.Muted)
	ok(w, map[string]bool{"muted": body.Muted})
}

func (s *HTTPServer) handleKey(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := SendKey(c, body.Key); err != nil {
		s.logger.Warn("key action failed key=%s err=%v", body.Key, err)
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("key action: %s", body.Key)
	ok(w, map[string]string{"key": body.Key})
}

func (s *HTTPServer) handleInput(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := SetInput(c, body.Input); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("input selected: %s", body.Input)
	ok(w, map[string]string{"input": body.Input})
}

// handleApp launches the named app on the TV.
func (s *HTTPServer) handleApp(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := LaunchApp(c, body.App); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("app launched: %s", body.App)
	ok(w, map[string]string{"app": body.App})
}

// handlePicture sets the TV's picture mode (e.g. cinema, vivid).
func (s *HTTPServer) handlePicture(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := SetPictureMode(c, body.Mode); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("picture mode: %s", body.Mode)
	ok(w, map[string]string{"mode": body.Mode})
}

// handleEnergy sets the TV's energy-saving level.
func (s *HTTPServer) handleEnergy(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := SetEnergySaving(c, body.Level); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("energy saving: %s", body.Level)
	ok(w, map[string]string{"level": body.Level})
}

// handleScreenMute blanks the TV screen or audio-only mode on or off.
func (s *HTTPServer) handleScreenMute(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := SetScreenMute(c, body.Mode); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logger.Info("screen mute: %s", body.Mode)
	ok(w, map[string]string{"mode": body.Mode})
}

// handleMacAddress fetches the TV's wired and wifi MAC addresses, optionally saving one.
func (s *HTTPServer) handleMacAddress(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	c, err := s.tvClient()
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	wired, wiredErr := GetMacAddress(c, "wired")
	wifi, wifiErr := GetMacAddress(c, "wifi")

	if wiredErr != nil && wifiErr != nil {
		fail(w, http.StatusBadGateway, "could not retrieve MAC addresses from TV")
		return
	}

	result := map[string]string{}
	if wiredErr == nil {
		result["wired"] = wired
	}
	if wifiErr == nil {
		result["wifi"] = wifi
	}

	// Optional: ?save=wired or ?save=wifi persists the chosen MAC to settings
	if save := r.URL.Query().Get("save"); save == "wired" || save == "wifi" {
		mac := result[save]
		if mac != "" {
			cfg := s.getCfg()
			cfg.TVMac = mac
			if err := SaveAppConfig(s.settingsPath, cfg); err == nil {
				s.setCfg(cfg)
				s.logger.Info("MAC address auto-saved (%s): %s", save, mac)
				result["saved"] = mac
			}
		}
	}

	ok(w, result)
}

func (s *HTTPServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	n := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	if n <= 0 || n > 5000 {
		n = 200
	}
	lines := s.logger.Tail(n)
	ok(w, map[string]any{"lines": lines})
}
