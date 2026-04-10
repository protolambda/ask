package ask

import "context"

// Option applies customizations to the runConfig, to adjust Run behavior.
type Option interface {
	Apply(cfg *runConfig)
}

type optionFn func(cfg *runConfig)

func (fn optionFn) Apply(cfg *runConfig) {
	fn(cfg)
}

// runConfig is the accumulation of options that are applied to change Run behavior.
type runConfig struct {
	ShowHidden bool

	OnDeprecated
}

func newRunConfig(opts ...Option) *runConfig {
	cfg := new(runConfig)
	for _, opt := range opts {
		opt.Apply(cfg)
	}
	return cfg
}

// Bundle bundles run options into a combined Option
func Bundle(opts ...Option) Option {
	return optionFn(func(cfg *runConfig) {
		for _, opt := range opts {
			opt.Apply(cfg)
		}
	})
}

// ShowHidden is an Option to show hidden flags
func ShowHidden(v bool) Option {
	return optionFn(func(cfg *runConfig) {
		cfg.ShowHidden = v
	})
}

// OnDeprecated is an Option to customize handling of deprecated flags.
// This function is called for each deprecated flag,
// and command execution exits immediately if this callback returns an error.
type OnDeprecated func(ctx context.Context, fl PrefixedFlag) error

func (fn OnDeprecated) Apply(cfg *runConfig) {
	cfg.OnDeprecated = fn
}
