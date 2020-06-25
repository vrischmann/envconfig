package envconfig_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vrischmann/envconfig"
)

type dateUnmarshaller time.Time

func (d *dateUnmarshaller) Unmarshal(s string) error {
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return err
	}
	*d = dateUnmarshaller(t)
	return nil
}

func TestParseStructWithUnmarshaler(t *testing.T) {
	test := func(conf *struct{ TestEnvDate *dateUnmarshaller }) {
		if assert.NoError(t, envconfig.Init(&conf)) {
			assert.Equal(t, time.Date(2020, time.June, 20, 0, 0, 0, 0, time.UTC), time.Time(*conf.TestEnvDate))
		}
	}

	os.Setenv("TEST_ENV_DATE", "2020-06-20")

	t.Run("val", func(t *testing.T) {
		conf := struct {
			TestEnvDate dateUnmarshaller
		}{}
		if assert.NoError(t, envconfig.Init(&conf)) {
			assert.Equal(t, time.Date(2020, time.June, 20, 0, 0, 0, 0, time.UTC), time.Time(conf.TestEnvDate))
		}
	})

	t.Run("nil", func(t *testing.T) {
		test(&struct{ TestEnvDate *dateUnmarshaller }{})
	})

	t.Run("nonnil", func(t *testing.T) {
		test(&struct{ TestEnvDate *dateUnmarshaller }{TestEnvDate: new(dateUnmarshaller)})
	})
}
