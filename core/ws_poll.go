package gnoduty

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dash "github.com/aviaone/gnoduty/v2/core/dashboard"
)

type pollBlockResult struct {
	Result struct {
		Block struct {
			Header struct {
				Height          string `json:"height"`
				ProposerAddress string `json:"proposer_address"`
			} `json:"header"`
			LastCommit struct {
				Precommits []struct {
					ValidatorAddress string `json:"validator_address"`
				} `json:"precommits"`
			} `json:"last_commit"`
		} `json:"block"`
	} `json:"result"`
}

func (cc *ChainConfig) PollRun() {
	started := time.Now()
	for {
		if cc.gnoRpcEndpoint == "" || cc.valInfo == nil {
			if started.Before(time.Now().Add(-2 * time.Minute)) {
				l(cc.name, "poller timed out waiting for endpoint")
				return
			}
			l("⏰ waiting for a healthy client for", cc.ChainId)
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	valAddr := cc.ValAddress
	l(fmt.Sprintf("⚙️ %-12s polling for new blocks from %s", cc.ChainId, cc.gnoRpcEndpoint))

	var lastHeight int64
	noBlockSince := time.Now()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			url := strings.TrimRight(cc.gnoRpcEndpoint, "/") + "/block"
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				if time.Since(noBlockSince) > 2*time.Minute {
					l("🛑", cc.ChainId, "no blocks for 2 min, exiting")
					return
				}
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var br pollBlockResult
			if err := json.Unmarshal(body, &br); err != nil {
				continue
			}

			var height int64
			fmt.Sscanf(br.Result.Block.Header.Height, "%d", &height)
			if height <= lastHeight {
				continue
			}
			noBlockSince = time.Now()
			lastHeight = height

			// Check signing status
			var signState StatusType = Statusmissed
			addrLower := strings.ToLower(valAddr)
			if strings.ToLower(br.Result.Block.Header.ProposerAddress) == addrLower {
				signState = StatusProposed
			} else {
				for _, sig := range br.Result.Block.LastCommit.Precommits {
					if strings.ToLower(sig.ValidatorAddress) == addrLower {
						signState = StatusSigned
						break
					}
				}
			}

			if height%20 == 0 {
				l(fmt.Sprintf("🧊 %-12s block %d", cc.ChainId, height))
			}

			cc.lastBlockNum = height
			if td.Prom {
				td.statsChan <- cc.mkUpdate(metricLastBlockSeconds, time.Since(cc.lastBlockTime).Seconds(), "")
			}
			cc.lastBlockTime = time.Now()
			cc.lastBlockAlarm = false
			info := getAlarms(cc.name)
			cc.blocksResults = append([]int{int(signState)}, cc.blocksResults[:len(cc.blocksResults)-1]...)

			if signState < 3 && cc.valInfo.Bonded {
				warn := fmt.Sprintf("❌ %s missed block %d on %s", cc.valInfo.Moniker, height, cc.ChainId)
				info += warn + "\n"
				cc.lastError = time.Now().UTC().String() + " " + info
				l(warn)
			}

			switch signState {
			case Statusmissed:
				cc.statTotalMiss += 1
				cc.statConsecutiveMiss += 1
				cc.valInfo.Missed += 1
			case StatusSigned:
				cc.statTotalSigns += 1
				cc.statConsecutiveMiss = 0
			case StatusProposed:
				cc.statTotalProps += 1
				cc.statTotalSigns += 1
				cc.statConsecutiveMiss = 0
			}
			cc.valInfo.Window += 1

			healthyNodes := 0
			for i := range cc.Nodes {
				if !cc.Nodes[i].down {
					healthyNodes += 1
				}
			}
			cc.activeAlerts = alarms.getCount(cc.name)
			if td.EnableDash {
				td.updateChan <- &dash.ChainStatus{
					MsgType:      "status",
					Name:         cc.name,
					ChainId:      cc.ChainId,
					Moniker:      cc.valInfo.Moniker,
					Bonded:       cc.valInfo.Bonded,
					Jailed:       cc.valInfo.Jailed,
					Tombstoned:   cc.valInfo.Tombstoned,
					Missed:       cc.valInfo.Missed,
					Window:       cc.valInfo.Window,
					Nodes:        len(cc.Nodes),
					HealthyNodes: healthyNodes,
					ActiveAlerts: cc.activeAlerts,
					Height:       height,
					LastError:    info,
					Blocks:       cc.blocksResults,
				}
			}
			if td.Prom {
				td.statsChan <- cc.mkUpdate(metricSigned, cc.statTotalSigns, "")
				td.statsChan <- cc.mkUpdate(metricProposed, cc.statTotalProps, "")
				td.statsChan <- cc.mkUpdate(metricMissed, cc.statTotalMiss, "")
				td.statsChan <- cc.mkUpdate(metricConsecutive, cc.statConsecutiveMiss, "")
				td.statsChan <- cc.mkUpdate(metricUnealthyNodes, float64(len(cc.Nodes)-healthyNodes), "")
			}

		case <-td.ctx.Done():
			return
		}
	}
}

