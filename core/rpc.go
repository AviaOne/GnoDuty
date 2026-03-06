package gnoduty

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	dash "github.com/aviaone/gnoduty/v2/core/dashboard"
)

// newRpc sets up the rpc client used for monitoring. It will try nodes in order until a working node is found.
// GnoDuty version: uses HTTP GET to /status instead of the Tendermint Go RPC client.
func (cc *ChainConfig) newRpc() error {
	var anyWorking bool
	for _, endpoint := range cc.Nodes {
		anyWorking = anyWorking || !endpoint.down
	}

	tryUrl := func(u string) (msg string, down, syncing bool) {
		_, err := url.Parse(u)
		if err != nil {
			msg = fmt.Sprintf("❌ could not parse url %s: (%s) %s", cc.name, u, err)
			l(msg)
			down = true
			return
		}

		// Use HTTP GET /status instead of Tendermint RPC client
		chainID, _, catchingUp, err := GnoGetStatus(u)
		if err != nil {
			msg = fmt.Sprintf("❌ could not get status for %s: (%s) %s", cc.name, u, err)
			down = true
			l(msg)
			return
		}
		if chainID != cc.ChainId {
			msg = fmt.Sprintf("chain id %s on %s does not match, expected %s, skipping", chainID, u, cc.ChainId)
			down = true
			l(msg)
			return
		}
		if catchingUp {
			msg = fmt.Sprint("🐢 node is not synced, skipping ", u)
			syncing = true
			down = true
			l(msg)
			return
		}

		// Store the working RPC URL for the websocket connection
		cc.gnoRpcEndpoint = u
		cc.noNodes = false
		return
	}

	down := func(endpoint *NodeConfig, msg string) {
		if !endpoint.down {
			endpoint.down = true
			endpoint.downSince = time.Now()
		}
		endpoint.lastMsg = msg
	}

	for _, endpoint := range cc.Nodes {
		if anyWorking && endpoint.down {
			continue
		}
		if msg, failed, syncing := tryUrl(endpoint.Url); failed {
			endpoint.syncing = syncing
			down(endpoint, msg)
			continue
		}
		return nil
	}

	cc.noNodes = true
	alarms.clearAll(cc.name)
	cc.lastError = "no usable RPC endpoints available for " + cc.ChainId
	if td.EnableDash {
		moniker := "not connected"
		if cc.valInfo != nil {
			moniker = cc.valInfo.Moniker
		}
		td.updateChan <- &dash.ChainStatus{
			MsgType:      "status",
			Name:         cc.name,
			ChainId:      cc.ChainId,
			Moniker:      moniker,
			Bonded:       cc.valInfo != nil && cc.valInfo.Bonded,
			Jailed:       cc.valInfo != nil && cc.valInfo.Jailed,
			Tombstoned:   cc.valInfo != nil && cc.valInfo.Tombstoned,
			Missed:       0,
			Window:       0,
			Nodes:        len(cc.Nodes),
			HealthyNodes: 0,
			ActiveAlerts: 1,
			Height:       0,
			LastError:    cc.lastError,
			Blocks:       cc.blocksResults,
		}
	}
	return errors.New("no usable endpoints available for " + cc.ChainId)
}

// monitorHealth periodically checks node health using HTTP /status.
func (cc *ChainConfig) monitorHealth(chainName string) {
	tick := time.NewTicker(time.Minute)

	for {
		select {
		case <-td.ctx.Done():
			return

		case <-tick.C:
			for _, node := range cc.Nodes {
				go func(node *NodeConfig) {
					alert := func(msg string) {
						node.lastMsg = fmt.Sprintf("%-12s node %s is %s", chainName, node.Url, msg)
						if !node.AlertIfDown {
							node.down = true
							return
						}
						if !node.down {
							node.down = true
							node.downSince = time.Now()
						}
						if td.Prom {
							td.statsChan <- cc.mkUpdate(metricNodeDownSeconds, time.Since(node.downSince).Seconds(), node.Url)
						}
						l("⚠️ " + node.lastMsg)
					}

					chainID, _, catchingUp, err := GnoGetStatus(node.Url)
					if err != nil {
						alert(err.Error())
						return
					}
					if chainID != cc.ChainId {
						alert("on the wrong network")
						return
					}
					if catchingUp {
						alert("not synced")
						node.syncing = true
						return
					}

					// node's OK, clear the note
					if node.down {
						node.lastMsg = ""
						node.wasDown = true
					}
					td.statsChan <- cc.mkUpdate(metricNodeDownSeconds, 0, node.Url)
					node.down = false
					node.syncing = false
					node.downSince = time.Unix(0, 0)
					cc.noNodes = false
					l(fmt.Sprintf("🟢 %-12s node %s is healthy", chainName, node.Url))
				}(node)
			}

			// Refresh validator info periodically
			if cc.valInfo != nil {
				cc.lastValInfo = &ValInfo{
					Moniker:    cc.valInfo.Moniker,
					Bonded:     cc.valInfo.Bonded,
					Jailed:     cc.valInfo.Jailed,
					Tombstoned: cc.valInfo.Tombstoned,
					Missed:     cc.valInfo.Missed,
					Window:     cc.valInfo.Window,
					Conspub:    cc.valInfo.Conspub,
					Valcons:    cc.valInfo.Valcons,
				}
			}
			err := cc.GnoGetValInfo(false)
			if err != nil {
				l("❓ refreshing signing info for", cc.ValAddress, err)
			}
		}
	}
}

func (c *Config) pingHealthcheck() {
	if !c.Healthcheck.Enabled {
		return
	}

	ticker := time.NewTicker(c.Healthcheck.PingRate * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				_, err := http.Get(c.Healthcheck.PingURL)
				if err != nil {
					l(fmt.Sprintf("❌ Failed to ping healthcheck URL: %s", err.Error()))
				} else {
					l(fmt.Sprintf("🏓 Successfully pinged healthcheck URL: %s", c.Healthcheck.PingURL))
				}
			}
		}
	}()
}

// endpointRex matches the first a tag's hostname and port if present.
var endpointRex = regexp.MustCompile(`//([^/:]+)(:\d+)?`)
