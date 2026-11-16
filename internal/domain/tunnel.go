package domain

import (
	"errors"
	"net"
	"strings"
)

var ErrTunnelNotFound = errors.New("tunnel not found")

type TunnelInput struct {
	Name          string `json:"name"`
	HostID        string `json:"hostID"`
	LocalAddress  string `json:"localAddress"`
	LocalPort     int    `json:"localPort"`
	TargetAddress string `json:"targetAddress"`
	TargetPort    int    `json:"targetPort"`
}

type Tunnel struct {
	TunnelInput
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func NormalizeTunnelInput(input TunnelInput) TunnelInput {
	input.Name = strings.TrimSpace(input.Name)
	input.HostID = strings.TrimSpace(input.HostID)
	input.LocalAddress = strings.Trim(strings.TrimSpace(input.LocalAddress), "[]")
	if input.LocalAddress == "" {
		input.LocalAddress = "127.0.0.1"
	}
	input.TargetAddress = strings.Trim(strings.TrimSpace(input.TargetAddress), "[]")
	return input
}

func ValidateTunnelInput(input TunnelInput) error {
	if input.Name == "" || len(input.Name) > 200 {
		return errors.New("enter a tunnel name of up to 200 bytes")
	}
	if input.HostID == "" {
		return errors.New("select an SSH host")
	}
	if net.ParseIP(input.LocalAddress) == nil {
		return errors.New("local listen address must be an IPv4 or IPv6 address")
	}
	if input.LocalPort < 1 || input.LocalPort > 65535 || input.TargetPort < 1 || input.TargetPort > 65535 {
		return errors.New("ports must be between 1 and 65535")
	}
	if input.TargetAddress == "" || len(input.TargetAddress) > 253 || strings.ContainsAny(input.TargetAddress, " /\\\t\r\n\x00") || (strings.Contains(input.TargetAddress, ":") && net.ParseIP(input.TargetAddress) == nil) {
		return errors.New("enter a target hostname or IP address without a port or URL scheme")
	}
	return nil
}
