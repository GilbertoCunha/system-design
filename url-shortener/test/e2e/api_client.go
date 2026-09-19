package tests

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/GilbertoCunha/system-design/url-shortener/internal"
)

type APIClient struct {
	BaseURL string
	Client  *http.Client
}

func NewAPIClient() *APIClient {
	return &APIClient{
		BaseURL: "http://localhost:8080", // TODO: port hard coded here
		Client:  http.DefaultClient,
	}
}

func (c *APIClient) GetLongUrl(shortUrl string) (*http.Response, *internal.LongUrl, error) {
	var longUrl *internal.LongUrl
	resp, err := c.Client.Get(
		c.BaseURL + "/api/v1/url/" + shortUrl,
	)
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		longUrl = &internal.LongUrl{}
		if err := json.NewDecoder(resp.Body).Decode(longUrl); err != nil {
			return nil, nil, err
		}
	}

	return resp, longUrl, err
}

func (c *APIClient) CreateShortUrl(url string) (*http.Response, *internal.ShortUrl, error) {
	body, err := json.Marshal(map[string]string{
		"longUrl": url,
	})
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.Client.Post(
		c.BaseURL+"/api/v1/url",
		"application/json",
		bytes.NewReader(body),
	)

	var shortUrl *internal.ShortUrl
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		shortUrl = &internal.ShortUrl{}
		if err := json.NewDecoder(resp.Body).Decode(shortUrl); err != nil {
			return nil, nil, err
		}
	}

	return resp, shortUrl, err
}
