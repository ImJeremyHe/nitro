package util

import (
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/providers/s3"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
)

func applyOverrides(f *flag.FlagSet, k *koanf.Koanf) error {
	// Apply command line options and environment variables
	if err := applyOverrideOverrides(f, k); err != nil {
		return err
	}

	// Load configuration file from S3 if setup
	if len(k.String("conf.s3.secret-key")) != 0 {
		if err := loadS3Variables(k); err != nil {
			return errors.Wrap(err, "error loading S3 settings")
		}

		if err := applyOverrideOverrides(f, k); err != nil {
			return err
		}
	}

	// Local config file overrides S3 config file
	configFile := k.String("conf.file")
	if len(configFile) > 0 {
		if err := k.Load(file.Provider(configFile), json.Parser()); err != nil {
			return errors.Wrap(err, "error loading local config file")
		}

		if err := applyOverrideOverrides(f, k); err != nil {
			return err
		}
	}

	return nil
}

// applyOverrideOverrides for configuration values that need to be re-applied for each configuration item applied
func applyOverrideOverrides(f *flag.FlagSet, k *koanf.Koanf) error {
	// Command line overrides config file or config string
	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		return errors.Wrap(err, "error loading command line config")
	}

	// Config string overrides any config file
	configString := k.String("conf.string")
	if len(configString) > 0 {
		if err := k.Load(rawbytes.Provider([]byte(configString)), json.Parser()); err != nil {
			return errors.Wrap(err, "error loading config string config")
		}

		// Command line overrides config file or config string
		if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
			return errors.Wrap(err, "error loading command line config")
		}
	}

	// Environment variables overrides config files or command line options
	if err := loadEnvironmentVariables(k); err != nil {
		return errors.Wrap(err, "error loading environment variables")
	}

	return nil
}

func loadEnvironmentVariables(k *koanf.Koanf) error {
	envPrefix := k.String("conf.env-prefix")
	if len(envPrefix) != 0 {
		return k.Load(env.Provider(envPrefix+"_", ".", func(s string) string {
			// FOO__BAR -> foo-bar to handle dash in config names
			s = strings.ReplaceAll(strings.ToLower(
				strings.TrimPrefix(s, envPrefix+"_")), "__", "-")
			return strings.ReplaceAll(s, "_", ".")
		}), nil)
	}

	return nil
}

func loadS3Variables(k *koanf.Koanf) error {
	return k.Load(s3.Provider(s3.Config{
		AccessKey: k.String("conf.s3.access-key"),
		SecretKey: k.String("conf.s3.secret-key"),
		Region:    k.String("conf.s3.region"),
		Bucket:    k.String("conf.s3.bucket"),
		ObjectKey: k.String("conf.s3.object-key"),
	}), nil)
}

func BeginCommonParse(f *flag.FlagSet, args []string) (*koanf.Koanf, error) {
	if err := f.Parse(args); err != nil {
		return nil, err
	}

	if f.NArg() != 0 {
		// Unexpected number of parameters
		return nil, errors.New("unexpected number of parameters")
	}

	var k = koanf.New(".")

	// Initial application of command line parameters and environment variables so other methods can be applied
	if err := applyOverrides(f, k); err != nil {
		return nil, err
	}

	return k, nil
}

func EndCommonParse(k *koanf.Koanf, config interface{}) error {
	decoderConfig := mapstructure.DecoderConfig{
		ErrorUnused: true,

		// Default values
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc()),
		Metadata:         nil,
		Result:           config,
		WeaklyTypedInput: true,
	}
	err := k.UnmarshalWithConf("", config, koanf.UnmarshalConf{DecoderConfig: &decoderConfig})
	if err != nil {
		return err
	}

	return nil
}