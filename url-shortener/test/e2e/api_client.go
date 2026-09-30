package tests

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/httpapi"
)

type APIClient struct {
	BaseURL string
	Client  *http.Client
}

func NewAPIClient() *APIClient {
	return &APIClient{
		BaseURL: "http://localhost:8080", // TODO: port hard coded here
		Client: &http.Client{
			// Return the redirect itself instead of following it, so tests can
			// assert on the 302 status and the Location header.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *APIClient) Health() (*http.Response, error) {
	return c.Client.Get(c.BaseURL + "/healthz")
}

func (c *APIClient) GetLongUrl(shortUrl string) (*http.Response, *httpapi.LongUrl, error) {
	var longUrl *httpapi.LongUrl
	resp, err := c.Client.Get(
		c.BaseURL + "/v1/url/" + shortUrl,
	)
	if err == nil && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		longUrl = &httpapi.LongUrl{LongUrl: resp.Header.Get("Location")}
	}

	return resp, longUrl, err
}

func (c *APIClient) CreateShortUrl(url string) (*http.Response, *httpapi.ShortUrl, error) {
	body, err := json.Marshal(map[string]string{
		"longUrl": url,
	})
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.Client.Post(
		c.BaseURL+"/v1/url",
		"application/json",
		bytes.NewReader(body),
	)

	var shortUrl *httpapi.ShortUrl
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		shortUrl = &httpapi.ShortUrl{}
		if err := json.NewDecoder(resp.Body).Decode(shortUrl); err != nil {
			return nil, nil, err
		}
	}

	return resp, shortUrl, err
}
