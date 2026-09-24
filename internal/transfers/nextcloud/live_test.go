package nextcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"bearstack/internal/transfers"
	"bearstack/internal/transfers/contracttest"
)

// Run only against a disposable Nextcloud instance. No test cleanup sends DELETE.
// SSL_CERT_FILE can supply the CA of an isolated HTTPS reverse proxy.
func TestNextcloudLive(t *testing.T) {
	server := os.Getenv("BEARSTACK_NEXTCLOUD_TEST_URL")
	if server == "" {
		t.Skip("set BEARSTACK_NEXTCLOUD_TEST_URL for isolated Nextcloud acceptance")
	}
	login, password := os.Getenv("BEARSTACK_NEXTCLOUD_TEST_USER"), os.Getenv("BEARSTACK_NEXTCLOUD_TEST_PASSWORD")
	if login == "" || password == "" {
		t.Fatal("isolated Nextcloud test credentials missing")
	}
	p := New()
	config, _ := json.Marshal(Config{Server: server})
	base, err := parseConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	auth := secret{Login: login, Password: password}
	u := endpoint(base, "/ocs/v2.php/cloud/user")
	u.RawQuery = "format=json"
	resp, err := p.request(t.Context(), "GET", u, &auth, nil, 0, map[string]string{"OCS-APIRequest": "true", "Accept": "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		OCS struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"ocs"`
	}
	if err = decodeJSON(resp, &identity); err != nil {
		t.Fatal(err)
	}
	auth.UserID = identity.OCS.Data.ID
	credentials, _ := json.Marshal(auth)
	adapter, err := p.Connect(t.Context(), config, credentials)
	if err != nil {
		t.Fatal(err)
	}
	c := adapter.(*client)
	makeFixture := func(t *testing.T) contracttest.Fixture {
		folder := "BearStack-acceptance-" + transfers.ID()
		if err := c.EnsureDirectories(t.Context(), transfers.Location{ID: "/"}, []string{folder}); err != nil {
			t.Fatal(err)
		}
		location := transfers.Location{ID: "/" + folder, Name: folder}
		read := func(parts []string) []byte {
			u, err := c.target(location, parts)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := p.request(context.Background(), "GET", u, &auth, nil, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode == 404 {
				return nil
			}
			if resp.StatusCode != 200 {
				t.Fatal(responseError(resp))
			}
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		return contracttest.Fixture{Client: c, Base: location, Read: read}
	}
	contracttest.Run(t, makeFixture)
	t.Run("ChunkedUpload", func(t *testing.T) {
		f := makeFixture(t)
		data := bytes.Repeat([]byte("x"), 21<<20)
		parts := []string{"large.mp4"}
		if err := f.Client.CreateFile(t.Context(), f.Base, contracttest.Upload(parts, data)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(f.Read(parts), data) {
			t.Fatal("chunked contents differ")
		}
		if err := f.Client.CreateFile(t.Context(), f.Base, contracttest.Upload(parts, data)); transfers.Kind(err) != transfers.Conflict {
			t.Fatalf("existing chunk destination not preserved: %v", err)
		}
	})
}
