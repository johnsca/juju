// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"io/ioutil"
	"os"
	"path"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/proxyupdater"
)

type ProxyUpdaterSuite struct {
	jujutesting.JujuConnSuite

	apiRoot        *api.State
	environmentAPI *environment.Facade
	machine        *state.Machine

	proxyFile string
	started   chan struct{}
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func (s *ProxyUpdaterSuite) setStarted() {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the environment API facade.
	s.environmentAPI = s.apiRoot.Environment()
	c.Assert(s.environmentAPI, gc.NotNil)

	proxyDir := c.MkDir()
	s.PatchValue(&proxyupdater.ProxyDirectory, proxyDir)
	s.started = make(chan struct{})
	s.PatchValue(&proxyupdater.Started, s.setStarted)
	s.PatchValue(&apt.ConfFile, path.Join(proxyDir, "juju-apt-proxy"))
	s.proxyFile = path.Join(proxyDir, proxyupdater.ProxyFile)
}

func (s *ProxyUpdaterSuite) waitForPostSetup(c *gc.C) {
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout while waiting for setup")
	case <-s.started:
	}
}

func (s *ProxyUpdaterSuite) waitProxySettings(c *gc.C, expected proxy.Settings) {
	for {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for proxy settings to change")
		case <-time.After(10 * time.Millisecond):
			obtained := proxy.DetectProxies()
			if obtained != expected {
				c.Logf("proxy settings are %#v, still waiting", obtained)
				continue
			}
			return
		}
	}
}

func (s *ProxyUpdaterSuite) waitForFile(c *gc.C, filename, expected string) {
	for {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for proxy settings to change")
		case <-time.After(10 * time.Millisecond):
			fileContent, err := ioutil.ReadFile(filename)
			if os.IsNotExist(err) {
				continue
			}
			c.Assert(err, jc.ErrorIsNil)
			if string(fileContent) != expected {
				c.Logf("file content not matching, still waiting")
				continue
			}
			return
		}
	}
}

func (s *ProxyUpdaterSuite) TestRunStop(c *gc.C) {
	updater := proxyupdater.New(s.environmentAPI, false)
	c.Assert(worker.Stop(updater), gc.IsNil)
}

func (s *ProxyUpdaterSuite) updateConfig(c *gc.C) (proxy.Settings, proxy.Settings) {

	proxySettings := proxy.Settings{
		Http:    "http proxy",
		Https:   "https proxy",
		Ftp:     "ftp proxy",
		NoProxy: "no proxy",
	}
	attrs := map[string]interface{}{}
	for k, v := range config.ProxyConfigMap(proxySettings) {
		attrs[k] = v
	}

	// We explicitly set apt proxy settings as well to show that it is the apt
	// settings that are used for the apt config, and not just the normal
	// proxy settings which is what we would get if we don't explicitly set
	// apt values.
	aptProxySettings := proxy.Settings{
		Http:  "apt http proxy",
		Https: "apt https proxy",
		Ftp:   "apt ftp proxy",
	}
	for k, v := range config.AptProxyConfigMap(aptProxySettings) {
		attrs[k] = v
	}

	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	return proxySettings, aptProxySettings
}

func (s *ProxyUpdaterSuite) TestInitialState(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, true)
	defer worker.Stop(updater)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")
	s.waitForFile(c, apt.ConfFile, apt.ProxyContent(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestWriteSystemFiles(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, true)
	defer worker.Stop(updater)
	s.waitForPostSetup(c)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")
	s.waitForFile(c, apt.ConfFile, apt.ProxyContent(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestDontWriteSystemFiles(c *gc.C) {
	proxySettings, _ := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, false)
	defer worker.Stop(updater)
	s.waitForPostSetup(c)

	s.waitProxySettings(c, proxySettings)
	c.Assert(apt.ConfFile, jc.DoesNotExist)
	c.Assert(s.proxyFile, jc.DoesNotExist)
}
