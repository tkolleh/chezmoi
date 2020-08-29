package cmd

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/twpayne/chezmoi/next/internal/chezmoi"
)

type passlikeConfig struct {
	Command  string
	cache    map[string]string
	unlocked bool
}

func init() {
	config.addTemplateFunc("gopass", func(id string) string {
		return config.passFunc(&config.Gopass, id)
	})
	config.addTemplateFunc("pass", func(id string) string {
		return config.passFunc(&config.Pass, id)
	})
}

func (c *Config) passFunc(passConfig *passlikeConfig, id string) string {
	if s, ok := passConfig.cache[id]; ok {
		return s
	}
	name := passConfig.Command
	if !passConfig.unlocked {
		args := []string{"grep", "^$"}
		cmd := exec.Command(name, args...)
		cmd.Stdin = c.stdin
		cmd.Stdout = c.stdout
		cmd.Stderr = c.stderr
		if err := cmd.Run(); err != nil {
			panic(fmt.Errorf("%s %s: %w", name, chezmoi.ShellQuoteArgs(args), err))
		}
		passConfig.unlocked = true
	}
	args := []string{"show", id}
	cmd := exec.Command(name, args...)
	output, err := c.baseSystem.IdempotentCmdOutput(cmd)
	if err != nil {
		panic(fmt.Errorf("%s %s: %w", name, chezmoi.ShellQuoteArgs(args), err))
	}
	var password string
	if index := bytes.IndexByte(output, '\n'); index != -1 {
		password = string(output[:index])
	} else {
		password = string(output)
	}
	if passConfig.cache == nil {
		passConfig.cache = make(map[string]string)
	}
	passConfig.cache[id] = password
	return password
}
