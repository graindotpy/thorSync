package syncthing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: &http.Client{Timeout: 70 * time.Second}}
}

func (c *Client) request(ctx context.Context, method, path string, body any, result any) error {
	if c.apiKey == "" {
		return errors.New("Syncthing API key is not configured")
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Syncthing %s %s: %s: %s", method, path, resp.Status, string(payload))
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) Health(ctx context.Context) error {
	return c.request(ctx, http.MethodGet, "/rest/noauth/health", nil, &struct {
		Status string `json:"status"`
	}{})
}

type Event struct {
	ID       int64           `json:"id"`
	GlobalID int64           `json:"globalID"`
	Type     string          `json:"type"`
	Time     time.Time       `json:"time"`
	Data     json.RawMessage `json:"data"`
}
type ChangeData struct {
	Folder     string `json:"folder"`
	Path       string `json:"path"`
	Action     string `json:"action"`
	Type       string `json:"type"`
	ModifiedBy string `json:"modifiedBy"`
}

func (c *Client) Events(ctx context.Context, since int64) ([]Event, error) {
	query := url.Values{}
	query.Set("since", fmt.Sprint(since))
	query.Set("timeout", "60")
	query.Set("events", "RemoteChangeDetected,LocalChangeDetected")
	var result []Event
	err := c.request(ctx, http.MethodGet, "/rest/events/disk?"+query.Encode(), nil, &result)
	return result, err
}

type FileVersion struct {
	Modified   time.Time `json:"modified"`
	ModifiedBy string    `json:"modifiedBy"`
	Size       int64     `json:"size"`
	Deleted    bool      `json:"deleted"`
	Name       string    `json:"name"`
}
type FileInfo struct {
	Global       FileVersion `json:"global"`
	Local        FileVersion `json:"local"`
	Availability []struct {
		ID            string `json:"id"`
		FromTemporary bool   `json:"fromTemporary"`
	} `json:"availability"`
}

func (c *Client) File(ctx context.Context, folder, path string) (FileInfo, error) {
	query := url.Values{}
	query.Set("folder", folder)
	query.Set("file", filepathSlash(path))
	var result FileInfo
	err := c.request(ctx, http.MethodGet, "/rest/db/file?"+query.Encode(), nil, &result)
	return result, err
}

type Device struct {
	DeviceID   string `json:"deviceID"`
	Name       string `json:"name"`
	Paused     bool   `json:"paused"`
	Introducer bool   `json:"introducer"`
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var result []Device
	err := c.request(ctx, http.MethodGet, "/rest/config/devices", nil, &result)
	return result, err
}

type Connections struct {
	Connections map[string]struct {
		Connected bool      `json:"connected"`
		Paused    bool      `json:"paused"`
		Address   string    `json:"address"`
		At        time.Time `json:"at"`
	} `json:"connections"`
}

func (c *Client) Connections(ctx context.Context) (Connections, error) {
	var result Connections
	err := c.request(ctx, http.MethodGet, "/rest/system/connections", nil, &result)
	return result, err
}

type FolderStatus struct {
	State        string    `json:"state"`
	StateChanged time.Time `json:"stateChanged"`
	NeedFiles    int       `json:"needFiles"`
	NeedBytes    int64     `json:"needBytes"`
	InSyncFiles  int       `json:"inSyncFiles"`
	Errors       int       `json:"errors"`
}

type CompletionStatus struct {
	Completion  float64 `json:"completion"`
	NeedBytes   int64   `json:"needBytes"`
	NeedItems   int     `json:"needItems"`
	NeedDeletes int     `json:"needDeletes"`
	RemoteState string  `json:"remoteState"`
}

func (c *Client) Completion(ctx context.Context, folder, device string) (CompletionStatus, error) {
	query := url.Values{}
	query.Set("folder", folder)
	query.Set("device", device)
	var result CompletionStatus
	err := c.request(ctx, http.MethodGet, "/rest/db/completion?"+query.Encode(), nil, &result)
	return result, err
}

func (c *Client) FolderStatus(ctx context.Context, folder string) (FolderStatus, error) {
	query := url.Values{}
	query.Set("folder", folder)
	var result FolderStatus
	err := c.request(ctx, http.MethodGet, "/rest/db/status?"+query.Encode(), nil, &result)
	return result, err
}

func (c *Client) Scan(ctx context.Context, folder string) error {
	query := url.Values{}
	query.Set("folder", folder)
	return c.request(ctx, http.MethodPost, "/rest/db/scan?"+query.Encode(), nil, nil)
}

type FolderConfig struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Paused  bool   `json:"paused"`
	Devices []struct {
		DeviceID string `json:"deviceID"`
	} `json:"devices"`
	FSWatcherEnabled bool           `json:"fsWatcherEnabled"`
	RescanIntervalS  int            `json:"rescanIntervalS"`
	Versioning       map[string]any `json:"versioning"`
}

func (c *Client) Folder(ctx context.Context, id string) (FolderConfig, error) {
	var result FolderConfig
	err := c.request(ctx, http.MethodGet, "/rest/config/folders/"+url.PathEscape(id), nil, &result)
	return result, err
}
func (c *Client) SetFolderPaused(ctx context.Context, id string, paused bool) error {
	folder, err := c.Folder(ctx, id)
	if err != nil {
		return err
	}
	folder.Paused = paused
	return c.request(ctx, http.MethodPut, "/rest/config/folders/"+url.PathEscape(id), folder, nil)
}

func (c *Client) EnsureFolder(ctx context.Context, id, label, path, deviceID string) error {
	folder, err := c.Folder(ctx, id)
	if err != nil {
		if !strings.Contains(err.Error(), "404") {
			return err
		}
		folder = FolderConfig{ID: id, Label: label, Path: path, Type: "sendreceive", FSWatcherEnabled: true, RescanIntervalS: 3600, Versioning: map[string]any{"type": "staggered", "params": map[string]string{"maxAge": "7776000", "cleanoutDays": "1"}, "cleanupIntervalS": 3600}}
	}
	if deviceID != "" {
		found := false
		for _, device := range folder.Devices {
			if device.DeviceID == deviceID {
				found = true
			}
		}
		if !found {
			folder.Devices = append(folder.Devices, struct {
				DeviceID string `json:"deviceID"`
			}{DeviceID: deviceID})
		}
	}
	return c.request(ctx, http.MethodPut, "/rest/config/folders/"+url.PathEscape(id), folder, nil)
}

func filepathSlash(path string) string { return strings.ReplaceAll(path, "\\", "/") }
