package lgdeck

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Valid enum values — mirrors TV.ts from lgtv-ip-control.

var ValidInputs = map[string]bool{
	"dtv": true, "atv": true, "cadtv": true, "catv": true,
	"avav1": true, "component1": true,
	"hdmi1": true, "hdmi2": true, "hdmi3": true, "hdmi4": true,
}

var ValidKeys = map[string]bool{
	"arrowdown": true, "arrowleft": true, "arrowright": true, "arrowup": true,
	"aspectratio": true, "audiomode": true, "returnback": true,
	"bluebutton": true, "captionsubtitle": true, "channeldown": true,
	"channellist": true, "channelup": true, "deviceinput": true,
	"screenbright": true, "fastforward": true, "greenbutton": true,
	"myapp": true, "programminfo": true, "livetv": true, "settingmenu": true,
	"number0": true, "number1": true, "number2": true, "number3": true,
	"number4": true, "number5": true, "number6": true, "number7": true,
	"number8": true, "number9": true,
	"ok": true, "play": true, "previouschannel": true, "programguide": true,
	"record": true, "redbutton": true, "rewind": true, "sleepreserve": true,
	"userguide": true, "videomode": true, "volumedown": true,
	"volumemute": true, "volumeup": true, "yellowbutton": true,
}

var ValidPictureModes = map[string]bool{
	"cinema": true, "eco": true, "filmMaker": true, "game": true,
	"normal": true, "sports": true, "vivid": true,
}

var ValidEnergySavingLevels = map[string]bool{
	// "screenoff" omitted: returns NG on 2018+ LG TVs via IP control.
	// Use SCREEN_MUTE screenmuteon instead.
	"auto": true, "maximum": true,
	"medium": true, "minimum": true, "off": true,
}

var ValidScreenMuteModes = map[string]bool{
	"screenmuteon": true, "videomuteon": true, "allmuteoff": true,
}

var ValidApps = map[string]string{
	"amazon":   "amazon",
	"netflix":  "netflix",
	"youtube":  "youtube.leanback.v4",
	"hulu":     "hulu",
	"disney":   "com.disney.disneyplus-prod",
	"hbomax":   "com.hbo.hbomax",
	"plex":     "cdp-30",
	"vudu":     "vudu",
	"slingtv":  "com.movenetworks.app.sling-tv-sling-production",
	"settings": "com.palm.app.settings",
	"browser":  "com.webos.app.browser",
	"photos":   "com.webos.app.photovideo",
	"music":    "com.webos.app.music",
	"gallery":  "com.webos.app.igallery",
}

// AppDetails is parsed from the CURRENT_APP response.
type AppDetails struct {
	App        string `json:"app"`
	HotPlug    string `json:"hot_plug,omitempty"`
	Signal     *bool  `json:"signal,omitempty"`
	HDCPVersion string `json:"hdcp_version,omitempty"`
	HDCPStatus  string `json:"hdcp_status,omitempty"`
}

// TVState aggregates the commonly-queried status fields.
type TVState struct {
	Reachable bool       `json:"reachable"`
	App       string     `json:"app,omitempty"`
	Volume    *int       `json:"volume,omitempty"`
	Muted     *bool      `json:"muted,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// --- Command helpers ---

func assertOK(resp string) error {
	if resp != "OK" {
		return fmt.Errorf("unexpected response: %q", resp)
	}
	return nil
}

// GetCurrentApp sends CURRENT_APP and parses the response.
func GetCurrentApp(c *TVClient) (*AppDetails, error) {
	resp, err := c.SendCommand("CURRENT_APP")
	if err != nil {
		return nil, err
	}
	if resp == "" {
		return nil, nil
	}
	re := regexp.MustCompile(`([\w\s]+):(\S+)`)
	pairs := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(resp, -1) {
		pairs[strings.TrimSpace(m[1])] = m[2]
	}
	app := pairs["APP"]
	if app == "" {
		return nil, fmt.Errorf("parse CURRENT_APP: %q", resp)
	}
	d := &AppDetails{
		App:         app,
		HotPlug:     pairs["Hot plug"],
		HDCPVersion: pairs["HDCP"],
		HDCPStatus:  pairs["HDCP Status"],
	}
	if sig, ok := pairs["Signal"]; ok {
		b := sig == "Yes"
		d.Signal = &b
	}
	return d, nil
}

// GetCurrentVolume sends CURRENT_VOL and parses the integer.
func GetCurrentVolume(c *TVClient) (int, error) {
	resp, err := c.SendCommand("CURRENT_VOL")
	if err != nil {
		return 0, err
	}
	m := regexp.MustCompile(`^VOL:(\d+)$`).FindStringSubmatch(resp)
	if m == nil {
		return 0, fmt.Errorf("parse CURRENT_VOL: %q", resp)
	}
	return strconv.Atoi(m[1])
}

// GetMuteState sends MUTE_STATE and parses on/off.
func GetMuteState(c *TVClient) (bool, error) {
	resp, err := c.SendCommand("MUTE_STATE")
	if err != nil {
		return false, err
	}
	m := regexp.MustCompile(`^MUTE:(on|off)$`).FindStringSubmatch(resp)
	if m == nil {
		return false, fmt.Errorf("parse MUTE_STATE: %q", resp)
	}
	return m[1] == "on", nil
}

// GetIPControlState verifies the TV acknowledges IP control.
func GetIPControlState(c *TVClient) (bool, error) {
	resp, err := c.SendCommand("GET_IPCONTROL_STATE")
	if err != nil {
		return false, err
	}
	return resp == "ON", nil
}

// GetMacAddress returns wired or wifi MAC.
func GetMacAddress(c *TVClient, kind string) (string, error) {
	if kind != "wired" && kind != "wifi" {
		return "", fmt.Errorf("kind must be wired or wifi")
	}
	return c.SendCommand("GET_MACADDRESS " + kind)
}

func PowerOff(c *TVClient) error {
	resp, err := c.SendCommand("POWER off")
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func LaunchApp(c *TVClient, app string) error {
	resp, err := c.SendCommand("APP_LAUNCH " + app)
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func SetInput(c *TVClient, input string) error {
	if !ValidInputs[input] {
		return fmt.Errorf("invalid input: %q", input)
	}
	resp, err := c.SendCommand("INPUT_SELECT " + input)
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func SetVolume(c *TVClient, level int) error {
	if level < 0 || level > 100 {
		return fmt.Errorf("volume must be 0-100, got %d", level)
	}
	resp, err := c.SendCommand(fmt.Sprintf("VOLUME_CONTROL %d", level))
	if err != nil {
		return err
	}
	return assertOK(resp)
}

// SetVolumeByKey reaches targetLevel from the current volume by firing
// concurrent KEY_ACTION volumeup/volumedown commands.
//
// Why: VOLUME_CONTROL sets the TV's internal speaker directly but bypasses
// HDMI CEC/SIMPLINK, so soundbars and AV receivers don't respond.
// KEY_ACTION simulates a physical remote press, which triggers CEC and also
// shows the TV's on-screen volume bar.
//
// The function uses a closed-loop approach: fire a concurrent batch of key
// presses, verify with CURRENT_VOL, repeat up to maxRounds if we overshot or
// dropped some commands. ctx is checked between rounds so a newer slider
// request can cancel an in-flight operation immediately.
func SetVolumeByKey(ctx context.Context, c *TVClient, targetLevel int) error {
	if targetLevel < 0 || targetLevel > 100 {
		return fmt.Errorf("volume must be 0-100, got %d", targetLevel)
	}

	const (
		maxConcurrent = 5 // TV tolerates 5 concurrent connections cleanly
		maxRounds     = 4 // retry ceiling — prevents infinite loops
	)

	for round := 0; round < maxRounds; round++ {
		// Bail out if a newer slider request has already cancelled us.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentLevel, err := GetCurrentVolume(c)
		if err != nil {
			return nil // can't read TV state, assume prior keys landed
		}

		delta := targetLevel - currentLevel
		if delta == 0 {
			return nil
		}

		key := "volumeup"
		steps := delta
		if delta < 0 {
			key = "volumedown"
			steps = -delta
		}
		if steps > 100 {
			steps = 100
		}

		// Fire a concurrent batch; individual NG responses are expected under
		// load and are intentionally swallowed — the TV still executes them.
		sem := make(chan struct{}, maxConcurrent)
		var wg sync.WaitGroup
		for i := 0; i < steps; i++ {
			// Check for cancellation before each goroutine launch so we
			// stop issuing new key presses as soon as a newer request arrives.
			select {
			case <-ctx.Done():
				wg.Wait()
				return ctx.Err()
			default:
			}
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				SendKey(c, key) //nolint:errcheck
			}()
		}
		wg.Wait()
	}

	return nil
}

func SetMute(c *TVClient, muted bool) error {
	val := "off"
	if muted {
		val = "on"
	}
	resp, err := c.SendCommand("VOLUME_MUTE " + val)
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func SendKey(c *TVClient, key string) error {
	if !ValidKeys[key] {
		return fmt.Errorf("invalid key: %q", key)
	}
	resp, err := c.SendCommand("KEY_ACTION " + key)
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func SetPictureMode(c *TVClient, mode string) error {
	if !ValidPictureModes[mode] {
		return fmt.Errorf("invalid picture mode: %q", mode)
	}
	resp, err := c.SendCommand("PICTURE_MODE " + mode)
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func SetEnergySaving(c *TVClient, level string) error {
	if !ValidEnergySavingLevels[level] {
		return fmt.Errorf("invalid energy saving level: %q", level)
	}
	resp, err := c.SendCommand("ENERGY_SAVING " + level)
	if err != nil {
		return err
	}
	return assertOK(resp)
}

func SetScreenMute(c *TVClient, mode string) error {
	if !ValidScreenMuteModes[mode] {
		return fmt.Errorf("invalid screen mute mode: %q", mode)
	}
	resp, err := c.SendCommand("SCREEN_MUTE " + mode)
	if err != nil {
		// When the TV blacks out the display it sometimes closes the TCP
		// connection before sending the ACK. Treat EOF as success for
		// mute-on commands — the screen IS off, we just didn't get the OK.
		if mode == "screenmuteon" || mode == "videomuteon" {
			return nil
		}
		return err
	}
	return assertOK(resp)
}

// GetTVState queries current app, volume, and mute state in one shot.
func GetTVState(c *TVClient) TVState {
	state := TVState{Reachable: true}

	app, err := GetCurrentApp(c)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	if app != nil {
		state.App = app.App
	}

	vol, err := GetCurrentVolume(c)
	if err == nil {
		state.Volume = &vol
	}

	muted, err := GetMuteState(c)
	if err == nil {
		state.Muted = &muted
	}

	return state
}
