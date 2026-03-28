package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"
)

type MgmtClient struct {
	BaseURL  string
	Username string
	Password string
	HTTP     *http.Client
}

type QueueInfo struct {
	Name      string `json:"name"`
	Messages  int64  `json:"messages"`
	Consumers int64  `json:"consumers"`
}

func NewMgmtClient(baseURL, username, password string) *MgmtClient {
	return &MgmtClient{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
		HTTP: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetQueueInfo 使用 RabbitMQ Management HTTP API 获取队列深度/消费者数。
// API: GET /api/queues/{vhost}/{name}
// vhost 默认是 "/"，需要 url-encode。
func (c *MgmtClient) GetQueueInfo(ctx context.Context, vhost, queue string) (QueueInfo, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return QueueInfo{}, err
	}

	// vhost="/" 在 URL path 里要变成 %2F
	vhostEsc := url.PathEscape(vhost)
	queueEsc := url.PathEscape(queue)
	base.Path = path.Join(base.Path, "/api/queues/", vhostEsc, queueEsc)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return QueueInfo{}, err
	}
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return QueueInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return QueueInfo{}, fmt.Errorf("mgmt api status=%d", resp.StatusCode)
	}

	var qi QueueInfo
	if err := json.NewDecoder(resp.Body).Decode(&qi); err != nil {
		return QueueInfo{}, err
	}
	return qi, nil
}
