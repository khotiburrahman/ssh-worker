package clashapi

import "time"

type ProxiesResponse struct {
	Proxies map[string]Proxy `json:"proxies"`
}

type Proxy struct {
	Type    string            `json:"type"`
	Name    string            `json:"name"`
	UDP     bool              `json:"udp"`
	History []DelayHistory    `json:"history,omitempty"`
	Now     string            `json:"now,omitempty"`
	All     []string          `json:"all,omitempty"`
	Alive   bool              `json:"alive,omitempty"`
	Extra   map[string]string `json:"extra,omitempty"`
}

type DelayHistory struct {
	Time  time.Time `json:"time"`
	Delay int       `json:"delay"`
}

type DelayResponse struct {
	Delay     int `json:"delay"`
	MeanDelay int `json:"meanDelay"`
}

type ConfigsResponse struct {
	Port        int    `json:"port"`
	SocksPort   int    `json:"socks-port"`
	RedirPort   int    `json:"redir-port"`
	MixedPort   int    `json:"mixed-port"`
	AllowLAN    bool   `json:"allow-lan"`
	BindAddress string `json:"bind-address"`
	Mode        string `json:"mode"`
	LogLevel    string `json:"log-level"`
	IPv6        bool   `json:"ipv6"`
}

type ConnectionsResponse struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
}

type Connection struct {
	ID          string   `json:"id"`
	Metadata    Metadata `json:"metadata"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Start       string   `json:"start"`
	Chains      []string `json:"chains"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
}

type Metadata struct {
	Network         string `json:"network"`
	Type            string `json:"type"`
	SourceIP        string `json:"sourceIP"`
	DestinationIP   string `json:"destinationIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
}

type TrafficResponse struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type LogMessage struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type VersionResponse struct {
	Version string `json:"version"`
	Meta    bool   `json:"meta,omitempty"`
}