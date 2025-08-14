package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if c.User == "" {
		return Config{}, fmt.Errorf("user is required")
	}
	if len(c.Lists) == 0 {
		return Config{}, fmt.Errorf("lists is required")
	}
	for i := range c.Lists {
		if c.Lists[i].Slug == "" {
			return Config{}, fmt.Errorf("list[%d].slug is required", i)
		}
		if c.Lists[i].MinStars < 0 {
			c.Lists[i].MinStars = 0
		}
		if c.Lists[i].StaleMonths == 0 {
			c.Lists[i].StaleMonths = 24
		}
	}
	return c, nil
}
