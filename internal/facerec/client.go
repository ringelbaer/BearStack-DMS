// Package facerec implements the bounded, private face inference protocol.
package facerec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const Model = "yunet-2023mar-sface-2021dec-v1"
const Dimensions = 128
const MaxFaces = 256
const MaxImageBytes = 8 << 20

type Detection struct {
	X          float64   `json:"x"`
	Y          float64   `json:"y"`
	Width      float64   `json:"width"`
	Height     float64   `json:"height"`
	Confidence float64   `json:"confidence"`
	Embedding  []float32 `json:"embedding"`
}
type Result struct {
	Model string      `json:"model"`
	Faces []Detection `json:"faces"`
}
type Health struct {
	Model    string `json:"model"`
	Ready    bool   `json:"ready"`
	Protocol int    `json:"protocol"`
}
type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func New(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(strings.TrimSpace(token)) < 32 || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("Gesichtsdienst: gültige HTTP(S)-Adresse und Token mit 32 bis 4096 Zeichen erforderlich")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // Private image traffic must not follow environment proxy settings.
	return &Client{strings.TrimRight(endpoint, "/"), token, &http.Client{Timeout: 60 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) request(ctx context.Context, method, path string, body []byte, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "image/jpeg")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("Gesichtsdienst nicht erreichbar")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gesichtsdienst: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return errors.New("ungültige Antwort des Gesichtsdienstes")
	}
	if err = json.Unmarshal(data, target); err != nil {
		return errors.New("ungültiges JSON des Gesichtsdienstes")
	}
	return nil
}
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var h Health
	if err := c.request(ctx, "GET", "/health", nil, &h); err != nil {
		return err
	}
	if !h.Ready || h.Protocol != 1 || h.Model != Model {
		return errors.New("Gesichtsdienst nicht bereit oder Modell inkompatibel")
	}
	return nil
}
func (c *Client) Analyze(ctx context.Context, jpeg []byte) (Result, error) {
	var result Result
	if len(jpeg) == 0 || len(jpeg) > MaxImageBytes {
		return result, errors.New("Bildgröße für Gesichtsdienst ungültig")
	}
	if err := c.request(ctx, "POST", "/v1/analyze", jpeg, &result); err != nil {
		return result, err
	}
	if result.Model != Model || result.Faces == nil || len(result.Faces) > MaxFaces {
		return Result{}, errors.New("inkompatible Gesichtsanalyse")
	}
	for i := range result.Faces {
		if err := Validate(&result.Faces[i]); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}
func Validate(d *Detection) error {
	for _, v := range []float64{d.X, d.Y, d.Width, d.Height, d.Confidence} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("ungültige Gesichtskoordinaten")
		}
	}
	if d.X < 0 || d.Y < 0 || d.Width <= 0 || d.Height <= 0 || d.X+d.Width > 1.000001 || d.Y+d.Height > 1.000001 || d.Confidence < 0 || d.Confidence > 1 || len(d.Embedding) != Dimensions {
		return errors.New("ungültige Gesichtsdaten")
	}
	var norm float64
	for _, v := range d.Embedding {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return errors.New("ungültiger Merkmalsvektor")
		}
		norm += float64(v) * float64(v)
	}
	if norm < 1e-12 || norm > 1e12 {
		return errors.New("ungültiger Merkmalsvektor")
	}
	norm = math.Sqrt(norm)
	for i := range d.Embedding {
		d.Embedding[i] /= float32(norm)
	}
	return nil
}
