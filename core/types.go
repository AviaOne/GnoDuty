package gnoduty

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	dash "github.com/aviaone/gnoduty/v2/core/dashboard"
	"github.com/go-yaml/yaml"
)

const (
	showBLocks = 512
	staleHours = 24
)

// Config holds both the settings for gnoduty to monitor and state information while running.
type Config struct {
	alertChan  chan *alertMsg
	updateChan chan *dash.ChainStatus
	logChan    chan dash.LogMessage
	statsChan  chan *promUpdate
	ctx        context.Context
	cancel     context.CancelFunc
	alarms     *alarmCache

	EnableDash bool   `yaml:"enable_dashboard"`
	Listen     string `yaml:"listen_port"`
	HideLogs   bool   `yaml:"hide_logs"`

	NodeDownMin      int    `yaml:"node_down_alert_minutes"`
	NodeDownSeverity string `yaml:"node_down_alert_severity"`

	Prom                 bool `yaml:"prometheus_enabled"`
	PrometheusListenPort int  `yaml:"prometheus_listen_port"`

	Pagerduty   PDConfig          `yaml:"pagerduty"`
	Discord     DiscordConfig     `yaml:"discord"`
	Telegram    TeleConfig        `yaml:"telegram"`
	Slack       SlackConfig       `yaml:"slack"`
	Healthcheck HealthcheckConfig `yaml:"healthcheck"`

	chainsMux sync.RWMutex
	Chains    map[string]*ChainConfig `yaml:"chains"`
}

type savedState struct {
	Alarms    *alarmCache                     `json:"alarms"`
	Blocks    map[string][]int                `json:"blocks"`
	NodesDown map[string]map[string]time.Time `json:"nodes_down"`
}

// ChainConfig represents a validator to be monitored on a chain.
type ChainConfig struct {
	name           string
	wsclient       *TmConn // custom websocket client
	gnoRpcEndpoint string  // GnoDuty: stores the working RPC URL
	noNodes        bool
	valInfo        *ValInfo
	lastValInfo    *ValInfo
	blocksResults  []int
	lastError      string
	lastBlockTime  time.Time
	lastBlockAlarm bool
	lastBlockNum   int64
	activeAlerts   int

	statTotalSigns      float64
	statTotalProps      float64
	statTotalMiss       float64
	statPrevoteMiss     float64
	statPrecommitMiss   float64
	statConsecutiveMiss float64

	ChainId    string `yaml:"chain_id"`
	ValAddress string `yaml:"valoper_address"`
	// GnoDuty: realm path for valopers, defaults to "gno.land/r/gnops/valopers"
	GnoValopersRealm string `yaml:"gno_valopers_realm"`
	ValconsOverride  string `yaml:"valcons_override"`
	ExtraInfo        string `yaml:"extra_info"`
	Alerts           AlertConfig `yaml:"alerts"`
	PublicFallback   bool   `yaml:"public_fallback"`
	Nodes            []*NodeConfig `yaml:"nodes"`
}

func (cc *ChainConfig) mkUpdate(t metricType, v float64, node string) *promUpdate {
	return &promUpdate{
		metric:   t,
		counter:  v,
		name:     cc.name,
		chainId:  cc.ChainId,
		moniker:  cc.valInfo.Moniker,
		endpoint: node,
	}
}

type AlertConfig struct {
	Stalled       int    `yaml:"stalled_minutes"`
	StalledAlerts bool   `yaml:"stalled_enabled"`

	ConsecutiveMissed   int    `yaml:"consecutive_missed"`
	ConsecutivePriority string `yaml:"consecutive_priority"`
	ConsecutiveAlerts   bool   `yaml:"consecutive_enabled"`

	Window             int    `yaml:"percentage_missed"`
	PercentagePriority string `yaml:"percentage_priority"`
	PercentageAlerts   bool   `yaml:"percentage_enabled"`

	AlertIfInactive  bool `yaml:"alert_if_inactive"`
	AlertIfNoServers bool `yaml:"alert_if_no_servers"`

	PagerdutyAlerts bool `yaml:"pagerduty_alerts"`
	DiscordAlerts   bool `yaml:"discord_alerts"`
	TelegramAlerts  bool `yaml:"telegram_alerts"`

	Pagerduty PDConfig      `yaml:"pagerduty"`
	Discord   DiscordConfig `yaml:"discord"`
	Telegram  TeleConfig    `yaml:"telegram"`
	Slack     SlackConfig   `yaml:"slack"`
}

type NodeConfig struct {
	Url         string `yaml:"url"`
	AlertIfDown bool   `yaml:"alert_if_down"`

	down      bool
	wasDown   bool
	syncing   bool
	lastMsg   string
	downSince time.Time
}

type PDConfig struct {
	Enabled         bool   `yaml:"enabled"`
	ApiKey          string `yaml:"api_key"`
	DefaultSeverity string `yaml:"default_severity"`
}

type DiscordConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Webhook  string   `yaml:"webhook"`
	Mentions []string `yaml:"mentions"`
}

type TeleConfig struct {
	Enabled  bool     `yaml:"enabled"`
	ApiKey   string   `yaml:"api_key"`
	Channel  string   `yaml:"channel"`
	Mentions []string `yaml:"mentions"`
}

type SlackConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Webhook  string   `yaml:"webhook"`
	Mentions []string `yaml:"mentions"`
}

type HealthcheckConfig struct {
	Enabled  bool          `yaml:"enabled"`
	PingURL  string        `yaml:"ping_url"`
	PingRate time.Duration `yaml:"ping_rate"`
}

func validateConfig(c *Config) (fatal bool, problems []string) {
	problems = make([]string, 0)
	var err error

	if c.EnableDash {
		_, err = url.Parse(c.Listen)
		if err != nil {
			fatal = true
			problems = append(problems, "could not parse listen URL: "+err.Error())
		}
	}

	for name, chain := range c.Chains {
		chain.name = name
		if chain.ChainId == "" {
			fatal = true
			problems = append(problems, fmt.Sprintf("chain %s has no chain_id", name))
		}
		if chain.ValAddress == "" {
			fatal = true
			problems = append(problems, fmt.Sprintf("chain %s has no valoper_address", name))
		}
		if len(chain.Nodes) == 0 && !chain.PublicFallback {
			fatal = true
			problems = append(problems, fmt.Sprintf("chain %s has no nodes configured and public_fallback is not enabled", name))
		}
		if chain.blocksResults == nil || len(chain.blocksResults) != showBLocks {
			chain.blocksResults = make([]int, showBLocks)
		}

		for _, n := range chain.Nodes {
			_, err = url.Parse(n.Url)
			if err != nil {
				fatal = true
				problems = append(problems, fmt.Sprintf("chain %s has an invalid node url: %s", name, n.Url))
			}
		}
	}

	return
}

func loadChainConfig(yamlFile string) (*ChainConfig, error) {
	//#nosec -- variable specified on command line
	f, e := os.OpenFile(yamlFile, os.O_RDONLY, 0600)
	if e != nil {
		return nil, e
	}
	i, e := f.Stat()
	if e != nil {
		_ = f.Close()
		return nil, e
	}
	b := make([]byte, int(i.Size()))
	_, e = f.Read(b)
	_ = f.Close()
	if e != nil {
		return nil, e
	}
	c := &ChainConfig{}
	e = yaml.Unmarshal(b, c)
	if e != nil {
		return nil, e
	}
	return c, nil
}

func loadConfig(yamlFile, stateFile, chainConfigDirectory string, password *string) (*Config, error) {
	c := &Config{}
	if strings.HasPrefix(yamlFile, "http://") || strings.HasPrefix(yamlFile, "https://") {
		if *password == "" {
			return nil, errors.New("a password is required if loading a remote configuration")
		}
		//#nosec -- url is specified on command line
		resp, err := http.Get(yamlFile)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		log.Printf("downloaded %d bytes from %s", len(b), yamlFile)
		decrypted, err := decrypt(b, *password)
		if err != nil {
			return nil, err
		}
		empty := ""
		password = &empty
		_ = os.Setenv("PASSWORD", "")
		err = yaml.Unmarshal(decrypted, c)
		if err != nil {
			return nil, err
		}
	} else {
		//#nosec -- variable specified on command line
		f, e := os.OpenFile(yamlFile, os.O_RDONLY, 0600)
		if e != nil {
			return nil, e
		}
		i, e := f.Stat()
		if e != nil {
			_ = f.Close()
			return nil, e
		}
		b := make([]byte, int(i.Size()))
		_, e = f.Read(b)
		_ = f.Close()
		if e != nil {
			return nil, e
		}
		e = yaml.Unmarshal(b, c)
		if e != nil {
			return nil, e
		}
	}

	// Load additional chain configuration files
	chainConfigFiles, e := os.ReadDir(chainConfigDirectory)
	if e != nil {
		l("Failed to scan chainConfigDirectory", e)
	}

	for _, chainConfigFile := range chainConfigFiles {
		if chainConfigFile.IsDir() {
			l("Skipping Directory: ", chainConfigFile.Name())
			continue
		}
		if !strings.HasSuffix(chainConfigFile.Name(), ".yml") {
			l("Skipping non .yml file: ", chainConfigFile.Name())
			continue
		}
		fmt.Println("Reading Chain Config File: ", chainConfigFile.Name())
		chainConfig, e := loadChainConfig(path.Join(chainConfigDirectory, chainConfigFile.Name()))
		if e != nil {
			l(fmt.Sprintf("Failed to read %s", chainConfigFile), e)
			return nil, e
		}
		chainName := strings.Split(chainConfigFile.Name(), ".")[0]
		if c.Chains == nil {
			c.Chains = make(map[string]*ChainConfig)
		}
		c.Chains[chainName] = chainConfig
		l(fmt.Sprintf("Added %s from ", chainName), chainConfigFile.Name())
	}

	if len(c.Chains) == 0 {
		return nil, errors.New("no chains configured")
	}

	c.alertChan = make(chan *alertMsg)
	c.logChan = make(chan dash.LogMessage)
	c.updateChan = make(chan *dash.ChainStatus, len(c.Chains)*2)
	c.statsChan = make(chan *promUpdate, len(c.Chains)*2)
	c.ctx, c.cancel = context.WithCancel(context.Background()) // #nosec

	c.alarms = &alarmCache{
		SentPdAlarms:  make(map[string]time.Time),
		SentTgAlarms:  make(map[string]time.Time),
		SentDiAlarms:  make(map[string]time.Time),
		SentSlkAlarms: make(map[string]time.Time),
		AllAlarms:     make(map[string]map[string]time.Time),
		notifyMux:     sync.RWMutex{},
	}

	//#nosec -- variable specified on command line
	sf, e := os.OpenFile(stateFile, os.O_RDONLY, 0600)
	if e != nil {
		l("could not load saved state", e.Error())
	}
	b, e := io.ReadAll(sf)
	_ = sf.Close()
	if e != nil {
		l("could not read saved state", e.Error())
	}
	saved := &savedState{}
	e = json.Unmarshal(b, saved)
	if e != nil {
		l("could not unmarshal saved state", e.Error())
	}
	for k, v := range saved.Blocks {
		if c.Chains[k] != nil {
			c.Chains[k].blocksResults = v
		}
	}

	if saved.Alarms != nil {
		if saved.Alarms.SentTgAlarms != nil {
			alarms.SentTgAlarms = saved.Alarms.SentTgAlarms
			clearStale(alarms.SentTgAlarms, "telegram", c.Pagerduty.Enabled, staleHours)
		}
		if saved.Alarms.SentPdAlarms != nil {
			alarms.SentPdAlarms = saved.Alarms.SentPdAlarms
			clearStale(alarms.SentPdAlarms, "PagerDuty", c.Pagerduty.Enabled, staleHours)
		}
		if saved.Alarms.SentDiAlarms != nil {
			alarms.SentDiAlarms = saved.Alarms.SentDiAlarms
			clearStale(alarms.SentDiAlarms, "Discord", c.Pagerduty.Enabled, staleHours)
		}
		if saved.Alarms.SentSlkAlarms != nil {
			alarms.SentSlkAlarms = saved.Alarms.SentSlkAlarms
			clearStale(alarms.SentSlkAlarms, "Slack", c.Pagerduty.Enabled, staleHours)
		}
		if saved.Alarms.AllAlarms != nil {
			alarms.AllAlarms = saved.Alarms.AllAlarms
			for _, alrm := range saved.Alarms.AllAlarms {
				clearStale(alrm, "dashboard", c.Pagerduty.Enabled, staleHours)
			}
		}
	}

	if saved.NodesDown != nil {
		for k, v := range saved.NodesDown {
			for nodeUrl := range v {
				if !v[nodeUrl].IsZero() {
					if c.Chains[k] != nil {
						for j := range c.Chains[k].Nodes {
							if c.Chains[k].Nodes[j].Url == nodeUrl {
								c.Chains[k].Nodes[j].down = true
								c.Chains[k].Nodes[j].wasDown = true
								c.Chains[k].Nodes[j].downSince = v[nodeUrl]
							}
						}
					}
				}
			}
		}
		for k, v := range c.Chains {
			downCount := 0
			for j := range v.Nodes {
				if v.Nodes[j].down {
					downCount += 1
				}
			}
			if downCount == len(c.Chains[k].Nodes) {
				c.Chains[k].noNodes = true
			}
		}
	}

	return c, nil
}

// Unused but kept for compatibility
var _ = regexp.MustCompile(`\W`)

func clearStale(alarms map[string]time.Time, what string, hasPagerduty bool, hours float64) {
	for k := range alarms {
		if time.Since(alarms[k]).Hours() >= hours {
			l(fmt.Sprintf("🗑 not restoring old alarm (%v >%.2f hours) from cache - %s", alarms[k], hours, k))
			if hasPagerduty && what == "pagerduty" {
				l("NOTE: stale alarms may need to be manually cleared from PagerDuty!")
			}
			delete(alarms, k)
			continue
		}
		l(fmt.Sprintf("📂 restored %s alarm state - %s", what, k))
	}
}
