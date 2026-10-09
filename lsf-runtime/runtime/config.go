package lsfrt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Config is a launch's configuration. Roles, rules and skills are input: the
// runtime has no built-in role, tool list or permission rule.
type Config struct {
	StateDir  string          `json:"stateDir"`
	Repo      string          `json:"repo"`
	ClaudeBin string          `json:"claudeBin,omitempty"` // default "claude"; env LSF_CLAUDE_BIN overrides
	Args      json.RawMessage `json:"args,omitempty"`      // passed through; clones read planSha, integrationBranch
	PluginDir string          `json:"pluginDir,omitempty"` // the plugin root; replaces <plugin> in roles' deny

	Concurrency int             `json:"concurrency,omitempty"` // default 4; bounds concurrent spawns
	Roles       map[string]Role `json:"roles"`
	RoleRules   []RoleRule      `json:"roleRules,omitempty"` // first match wins
	DefaultRole string          `json:"defaultRole"`
	Skills      []SkillRule     `json:"skills,omitempty"`

	Caps                 Caps         `json:"caps,omitzero"`
	GuardTimeout         Duration     `json:"guardTimeout,omitempty"` // 0 = none
	KillGrace            Duration     `json:"killGrace,omitempty"`    // default 10s
	Backoff              []Duration   `json:"backoff,omitempty"`      // default [5s, 20s, 60s]
	SessionLimitMargin   Duration     `json:"sessionLimitMargin,omitempty"`
	SessionLimitFallback Duration     `json:"sessionLimitFallback,omitempty"`
	MaxSessionWaits      int          `json:"maxSessionWaits,omitempty"`
	DryRun               bool         `json:"dryRun,omitempty"`
	GateTimeout          Duration     `json:"gateTimeout,omitempty"`     // default 2h; a declared gate's own timeout overrides it
	GateConcurrency      int          `json:"gateConcurrency,omitempty"` // default 1; bounds concurrent RunGates
	Proxy                *ProxyConfig `json:"proxy,omitempty"`           // the credential proxy; none without it

	roleRes  []*regexp.Regexp // compiled RoleRules[i].Label, nil when empty
	skillRes []*regexp.Regexp // compiled Skills[i].Label
	prepared bool
}

// Role is the toolset and permissions a call runs with.
type Role struct {
	Tools          string   `json:"tools"`           // --tools, verbatim
	Allow          []string `json:"allow,omitempty"` // --allowedTools
	Deny           []string `json:"deny,omitempty"`  // --disallowedTools
	PermissionMode string   `json:"permissionMode"`  // --permission-mode
	Clone          string   `json:"clone"`           // "kept" (fetched back) or "discarded" (removed after the call)
}

// Role.Clone values.
const (
	CloneKept      = "kept"
	CloneDiscarded = "discarded"
)

// pluginToken in a role's deny entry is replaced by Config.PluginDir.
const pluginToken = "<plugin>"

// RoleRule matches when every non-empty matcher matches (AND); at least one is
// set. Express OR as two rules. Label is an RE2 regex, unanchored: anchor it
// with ^ to match a label prefix.
type RoleRule struct {
	AgentType string `json:"agentType,omitempty"`
	Label     string `json:"label,omitempty"`
	Role      string `json:"role"`
}

// SkillRule sends a matching label's prompt as the skill's slash command.
// Label is an unanchored RE2 regex.
type SkillRule struct {
	Label string `json:"label"`
	Skill string `json:"skill"`
}

// Caps bound a launch; zero means no cap.
type Caps struct {
	MaxCalls     int      `json:"maxCalls,omitempty"`
	MaxWallClock Duration `json:"maxWallClock,omitempty"`
	MaxCostUSD   float64  `json:"maxCostUSD,omitempty"`
}

// Duration is a time.Duration written in JSON as a Go duration string ("90s").
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string such as \"90s\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

// LoadConfig reads a config file strictly: unknown fields are errors.
func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if dec.More() {
		return Config{}, fmt.Errorf("config %s: trailing data after the config object", path)
	}
	if err := c.prepare(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

// prepare validates the config, compiles its regexes and fills defaults.
func (c *Config) prepare() error {
	if c.prepared {
		return nil
	}
	var errs []error
	if c.StateDir == "" {
		errs = append(errs, errors.New("stateDir is required"))
	}
	if len(c.Roles) == 0 {
		errs = append(errs, errors.New("roles: at least one role is required"))
	}
	for _, name := range slices.Sorted(maps.Keys(c.Roles)) {
		r := c.Roles[name]
		if r.PermissionMode == "" {
			errs = append(errs, fmt.Errorf("roles.%s: permissionMode is required", name))
		}
		if r.Tools == "" {
			errs = append(errs, fmt.Errorf("roles.%s: tools is required", name))
		}
		if r.Clone != CloneKept && r.Clone != CloneDiscarded {
			errs = append(errs, fmt.Errorf("roles.%s: clone must be %q or %q, not %q", name, CloneKept, CloneDiscarded, r.Clone))
		}
		hasToken := func(d string) bool { return strings.Contains(d, pluginToken) }
		if c.PluginDir == "" && slices.ContainsFunc(r.Deny, hasToken) {
			errs = append(errs, fmt.Errorf("roles.%s: deny names %s but pluginDir is not set", name, pluginToken))
		}
		if slices.ContainsFunc(r.Allow, hasToken) {
			errs = append(errs, fmt.Errorf("roles.%s: %s is substituted in deny only, not allow", name, pluginToken))
		}
	}
	if _, ok := c.Roles[c.DefaultRole]; !ok {
		errs = append(errs, fmt.Errorf("defaultRole %q names no role", c.DefaultRole))
	}
	c.roleRes = make([]*regexp.Regexp, len(c.RoleRules))
	for i, r := range c.RoleRules {
		if r.AgentType == "" && r.Label == "" {
			errs = append(errs, fmt.Errorf("roleRules[%d]: set agentType, label or both", i))
		}
		if _, ok := c.Roles[r.Role]; !ok {
			errs = append(errs, fmt.Errorf("roleRules[%d]: role %q names no role", i, r.Role))
		}
		if r.Label != "" {
			re, err := regexp.Compile(r.Label)
			if err != nil {
				errs = append(errs, fmt.Errorf("roleRules[%d].label: %w", i, err))
			}
			c.roleRes[i] = re
		}
	}
	c.skillRes = make([]*regexp.Regexp, len(c.Skills))
	for i, s := range c.Skills {
		if s.Skill == "" {
			errs = append(errs, fmt.Errorf("skills[%d]: skill is required", i))
		}
		re, err := regexp.Compile(s.Label)
		switch {
		case s.Label == "":
			errs = append(errs, fmt.Errorf("skills[%d].label is required", i))
		case err != nil:
			errs = append(errs, fmt.Errorf("skills[%d].label: %w", i, err))
		}
		c.skillRes[i] = re
	}
	if c.Proxy != nil {
		errs = append(errs, c.Proxy.prepare()...)
	}
	if c.Concurrency < 0 || c.GateConcurrency < 0 || c.MaxSessionWaits < 0 || c.Caps.MaxCalls < 0 || c.Caps.MaxCostUSD < 0 {
		errs = append(errs, errors.New("concurrency, gateConcurrency, maxSessionWaits and caps must not be negative"))
	}
	durations := []Duration{c.GuardTimeout, c.KillGrace, c.SessionLimitMargin, c.SessionLimitFallback, c.Caps.MaxWallClock, c.GateTimeout}
	durations = append(durations, c.Backoff...)
	for _, d := range durations {
		if d < 0 {
			errs = append(errs, errors.New("durations must not be negative"))
			break
		}
	}
	if c.Backoff != nil && len(c.Backoff) == 0 {
		errs = append(errs, errors.New("backoff: omit it for the default; an empty list would retry transient errors never"))
	}
	for _, p := range []*string{&c.StateDir, &c.Repo, &c.PluginDir} {
		if *p != "" {
			abs, err := filepath.Abs(*p)
			if err != nil {
				errs = append(errs, err)
			}
			*p = abs
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if c.ClaudeBin == "" {
		c.ClaudeBin = "claude"
	}
	if v := os.Getenv("LSF_CLAUDE_BIN"); v != "" {
		c.ClaudeBin = v
	}
	if c.Concurrency == 0 {
		c.Concurrency = 4
	}
	if c.KillGrace == 0 {
		c.KillGrace = Duration(10 * time.Second)
	}
	if c.Backoff == nil {
		c.Backoff = []Duration{Duration(5 * time.Second), Duration(20 * time.Second), Duration(60 * time.Second)}
	}
	if c.SessionLimitMargin == 0 {
		c.SessionLimitMargin = Duration(30 * time.Second)
	}
	if c.SessionLimitFallback == 0 {
		c.SessionLimitFallback = Duration(15 * time.Minute)
	}
	if c.GateTimeout == 0 {
		c.GateTimeout = Duration(2 * time.Hour)
	}
	if c.GateConcurrency == 0 {
		c.GateConcurrency = 1
	}
	if c.MaxSessionWaits == 0 {
		c.MaxSessionWaits = 4
	}
	c.prepared = true
	return nil
}

// roleFor picks the call's role: the first matching rule, else the default.
func (c *Config) roleFor(o Opts) (string, Role) {
	for i, r := range c.RoleRules {
		if r.AgentType != "" && r.AgentType != o.AgentType {
			continue
		}
		if c.roleRes[i] != nil && !c.roleRes[i].MatchString(o.Label) {
			continue
		}
		return r.Role, c.Roles[r.Role]
	}
	return c.DefaultRole, c.Roles[c.DefaultRole]
}

// skillFor names the skill a label's prompt is sent as, or "".
func (c *Config) skillFor(label string) string {
	for i, s := range c.Skills {
		if c.skillRes[i].MatchString(label) {
			return s.Skill
		}
	}
	return ""
}
