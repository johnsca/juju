package deployer_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/deployer"
)

type SimpleContextSuite struct {
	SimpleToolsFixture
}

var _ = Suite(&SimpleContextSuite{})

func (s *SimpleContextSuite) SetUpTest(c *C) {
	s.SimpleToolsFixture.SetUp(c, c.MkDir())
}

func (s *SimpleContextSuite) TearDownTest(c *C) {
	s.SimpleToolsFixture.TearDown(c)
}

func (s *SimpleContextSuite) TestDeployRecall(c *C) {
	mgr0 := s.getContext(c, "test-entity-0")
	units, err := mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	s.assertUpstartCount(c, 0)

	err = mgr0.DeployUnit("foo/123", "some-password")
	c.Assert(err, IsNil)
	units, err = mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []string{"foo/123"})
	s.assertUpstartCount(c, 1)
	s.checkUnitInstalled(c, "foo/123", "test-entity-0", "some-password")

	err = mgr0.RecallUnit("foo/123")
	c.Assert(err, IsNil)
	units, err = mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	s.assertUpstartCount(c, 0)
	s.checkUnitRemoved(c, "foo/123", "test-entity-0")
}

func (s *SimpleContextSuite) TestIndependentManagers(c *C) {
	mgr0 := s.getContext(c, "test-entity-0")
	err := mgr0.DeployUnit("foo/123", "some-password")
	c.Assert(err, IsNil)

	mgr1 := s.getContext(c, "test-entity-1")
	units, err := mgr1.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)

	err = mgr1.RecallUnit("foo/123")
	c.Assert(err, ErrorMatches, `unit "foo/123" is not deployed`)
	s.checkUnitInstalled(c, "foo/123", "test-entity-0", "some-password")

	units, err = mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []string{"foo/123"})
	s.assertUpstartCount(c, 1)
	s.checkUnitInstalled(c, "foo/123", "test-entity-0", "some-password")
}

type SimpleToolsFixture struct {
	dataDir  string
	initDir  string
	logDir   string
	origPath string
	binDir   string
}

var fakeJujud = "#!/bin/bash\n# fake-jujud\nexit 0\n"

func (fix *SimpleToolsFixture) SetUp(c *C, dataDir string) {
	fix.dataDir = dataDir
	fix.initDir = c.MkDir()
	fix.logDir = c.MkDir()
	toolsDir := agent.SharedToolsDir(fix.dataDir, version.Current)
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, IsNil)
	jujudPath := filepath.Join(toolsDir, "jujud")
	err = ioutil.WriteFile(jujudPath, []byte(fakeJujud), 0755)
	c.Assert(err, IsNil)
	urlPath := filepath.Join(toolsDir, "downloaded-url.txt")
	err = ioutil.WriteFile(urlPath, []byte("http://example.com/tools"), 0644)
	c.Assert(err, IsNil)
	fix.binDir = c.MkDir()
	fix.origPath = os.Getenv("PATH")
	os.Setenv("PATH", fix.binDir+":"+fix.origPath)
	fix.makeBin(c, "status", `echo "blah stop/waiting"`)
	fix.makeBin(c, "stopped-status", `echo "blah stop/waiting"`)
	fix.makeBin(c, "started-status", `echo "blah start/running, process 666"`)
	fix.makeBin(c, "start", "cp $(which started-status) $(which status)")
	fix.makeBin(c, "stop", "cp $(which stopped-status) $(which status)")
}

func (fix *SimpleToolsFixture) TearDown(c *C) {
	os.Setenv("PATH", fix.origPath)
}

func (fix *SimpleToolsFixture) makeBin(c *C, name, script string) {
	path := filepath.Join(fix.binDir, name)
	err := ioutil.WriteFile(path, []byte("#!/bin/bash\n"+script), 0755)
	c.Assert(err, IsNil)
}

func (fix *SimpleToolsFixture) assertUpstartCount(c *C, count int) {
	fis, err := ioutil.ReadDir(fix.initDir)
	c.Assert(err, IsNil)
	c.Assert(fis, HasLen, count)
}

func (fix *SimpleToolsFixture) getContext(c *C, deployerName string) *deployer.SimpleContext {
	return deployer.NewTestSimpleContext(deployerName, fix.initDir, fix.dataDir, fix.logDir)
}

func (fix *SimpleToolsFixture) paths(tag, xName string) (confPath, agentDir, toolsDir string) {
	confName := fmt.Sprintf("jujud-%s:%s.conf", xName, tag)
	confPath = filepath.Join(fix.initDir, confName)
	agentDir = agent.Dir(fix.dataDir, tag)
	toolsDir = agent.ToolsDir(fix.dataDir, tag)
	return
}

func (fix *SimpleToolsFixture) checkUnitInstalled(c *C, name, xName, password string) {
	tag := state.UnitTag(name)
	uconfPath, _, toolsDir := fix.paths(tag, xName)
	uconfData, err := ioutil.ReadFile(uconfPath)
	c.Assert(err, IsNil)
	uconf := string(uconfData)
	var execLine string
	for _, line := range strings.Split(uconf, "\n") {
		if strings.HasPrefix(line, "exec ") {
			execLine = line
			break
		}
	}
	if execLine == "" {
		c.Fatalf("no command found in %s:\n%s", uconfPath, uconf)
	}
	logPath := filepath.Join(fix.logDir, tag+".log")
	jujudPath := filepath.Join(toolsDir, "jujud")
	for _, pat := range []string{
		"^exec " + jujudPath + " unit ",
		" --unit-name " + name + " ",
		" >> " + logPath + " 2>&1$",
	} {
		match, err := regexp.MatchString(pat, execLine)
		c.Assert(err, IsNil)
		if !match {
			c.Fatalf("failed to match:\n%s\nin:\n%s", pat, execLine)
		}
	}

	conf, err := agent.ReadConf(fix.dataDir, tag)
	c.Assert(err, IsNil)
	c.Assert(conf, DeepEquals, &agent.Conf{
		DataDir:     fix.dataDir,
		OldPassword: password,
		StateInfo: &state.Info{
			Addrs:  []string{"s1:123", "s2:123"},
			CACert: []byte("test-cert"),
			Tag:    tag,
		},
	})

	jujudData, err := ioutil.ReadFile(jujudPath)
	c.Assert(err, IsNil)
	c.Assert(string(jujudData), Equals, fakeJujud)
}

func (fix *SimpleToolsFixture) checkUnitRemoved(c *C, name, xName string) {
	tag := state.UnitTag(name)
	confPath, agentDir, toolsDir := fix.paths(tag, xName)
	for _, path := range []string{confPath, agentDir, toolsDir} {
		_, err := ioutil.ReadFile(path)
		c.Assert(os.IsNotExist(err), Equals, true)
	}
}
