package rules_test

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/ergomodel"
	"ergo.tools/argus/rules"
	"ergo.tools/argus/rules/alwaysfails"
	"ergo.tools/argus/rules/appinitleak"
	"ergo.tools/argus/rules/behaviorspec"
	"ergo.tools/argus/rules/callbackblocking"
	"ergo.tools/argus/rules/callbackgoroutine"
	"ergo.tools/argus/rules/callbackroundtrip"
	"ergo.tools/argus/rules/callbudget"
	"ergo.tools/argus/rules/deferredidentity"
	"ergo.tools/argus/rules/doublereply"
	"ergo.tools/argus/rules/eventbuffer"
	"ergo.tools/argus/rules/eventnotify"
	"ergo.tools/argus/rules/eventprotocol"
	"ergo.tools/argus/rules/goroutinerecover"
	"ergo.tools/argus/rules/handlecallreason"
	"ergo.tools/argus/rules/housethreshold"
	"ergo.tools/argus/rules/initbudget"
	"ergo.tools/argus/rules/initsentinel"
	"ergo.tools/argus/rules/intentcomment"
	"ergo.tools/argus/rules/internalescape"
	"ergo.tools/argus/rules/logreturn"
	"ergo.tools/argus/rules/messagealiasing"
	"ergo.tools/argus/rules/messagemarker"
	"ergo.tools/argus/rules/metastart"
	"ergo.tools/argus/rules/metastateshare"
	"ergo.tools/argus/rules/nodesend"
	"ergo.tools/argus/rules/noreply"
	"ergo.tools/argus/rules/ownstatesend"
	"ergo.tools/argus/rules/registration"
	"ergo.tools/argus/rules/registrylist"
	"ergo.tools/argus/rules/selfrequest"
	"ergo.tools/argus/rules/spawnargs"
	"ergo.tools/argus/rules/stategate"
	"ergo.tools/argus/rules/supervisorspec"
	"ergo.tools/argus/rules/suppressiondebt"
	"ergo.tools/argus/rules/terminatehalfbuilt"
	"ergo.tools/argus/rules/terminateio"
	"ergo.tools/argus/rules/timerchain"
	"ergo.tools/argus/rules/timermisuse"
	"ergo.tools/argus/rules/uncheckedresult"
	"ergo.tools/argus/rules/webrequest"
	"ergo.tools/argus/rules/wireerror"
	"ergo.tools/argus/rules/wiresentinel"
	"ergo.tools/argus/rules/wireshape"
	"ergo.tools/argus/rules/wiretypeclosure"
)

func testdata(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../testdata")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

var ruleCases = []struct {
	analyzer *analysis.Analyzer
	pkg      string
}{
	{messagealiasing.Analyzer, "a1001"},
	{callbackblocking.Analyzer, "a1002"},
	{selfrequest.Analyzer, "a1003"},
	{callbackgoroutine.Analyzer, "a1004"},
	{internalescape.Analyzer, "a1007"},
	{handlecallreason.Analyzer, "a1008"},
	{goroutinerecover.Analyzer, "a1010"},
	{initbudget.Analyzer, "a1011"},
	{stategate.Analyzer, "a2001"},
	{uncheckedresult.Analyzer, "a2001a"},
	{noreply.Analyzer, "a2002"},
	{doublereply.Analyzer, "a2003"},
	{wireshape.Analyzer, "a2004"},
	{supervisorspec.Analyzer, "a2005"},
	{timermisuse.Analyzer, "a2006"},
	{registration.Analyzer, "a2009"},
	{spawnargs.Analyzer, "a2011"},
	{logreturn.Analyzer, "a2012"},
	{eventprotocol.Analyzer, "a2007"},
	{webrequest.Analyzer, "a2008"},
	{eventbuffer.Analyzer, "a2013"},
	{terminateio.Analyzer, "a2014"},
	{terminatehalfbuilt.Analyzer, "a2014a"},
	{timerchain.Analyzer, "a2015"},
	{metastateshare.Analyzer, "a1005"},
	{ownstatesend.Analyzer, "a1006"},
	{nodesend.Analyzer, "a1012"},
	{deferredidentity.Analyzer, "a2016"},
	{initsentinel.Analyzer, "a2017"},
	{eventnotify.Analyzer, "a2018"},
	{messagemarker.Analyzer, "a3001"},
	{registrylist.Analyzer, "a3003"},
	{suppressiondebt.Analyzer, "a3005"},
	{intentcomment.Analyzer, "a3006"},
	{housethreshold.Analyzer, "a3007"},
	{wireerror.Analyzer, "a2020"},
	{wiretypeclosure.Analyzer, "a2021"},
	{wiresentinel.Analyzer, "a2022"},
	{appinitleak.Analyzer, "a2023"},
	{callbackroundtrip.Analyzer, "a2024"},
	{behaviorspec.Analyzer, "a2025"},
	{alwaysfails.Analyzer, "a2026"},
	{metastart.Analyzer, "a2027"},
	{callbudget.Analyzer, "a2028"},
}

func TestRules(t *testing.T) {
	dir := testdata(t)
	for _, c := range ruleCases {
		t.Run(c.analyzer.Name, func(t *testing.T) {
			analysistest.Run(t, dir, c.analyzer, c.pkg)
		})
	}
}

func TestSuiteConcurrent(t *testing.T) {
	dir := testdata(t)
	done := make(chan struct{}, len(ruleCases))
	for _, c := range ruleCases {
		go func(a *analysis.Analyzer, pkg string) {
			defer func() { done <- struct{}{} }()
			analysistest.Run(t, dir, a, pkg)
		}(c.analyzer, c.pkg)
	}
	for range ruleCases {
		<-done
	}
}

func TestEveryRuleHasAFixture(t *testing.T) {
	covered := map[string]bool{}
	for _, c := range ruleCases {
		covered[c.analyzer.Name] = true
	}
	for _, a := range rules.Rules() {
		if covered[a.Name] == false {
			t.Errorf("%s is in Suite but has no fixture in ruleCases", a.Name)
		}
	}
}

func TestSuiteIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range rules.Rules() {
		if a.Doc == "" {
			t.Errorf("%s: empty Doc", a.Name)
		}
		if a.URL == "" {
			t.Errorf("%s: empty URL", a.Name)
		}
		if seen[a.Name] {
			t.Errorf("%s: duplicate analyzer name", a.Name)
		}
		seen[a.Name] = true

		requiresModel := false
		for _, r := range a.Requires {
			if r == ergomodel.Analyzer {
				requiresModel = true
			}
		}
		if requiresModel == false {
			t.Errorf("%s: does not require the model analyzer", a.Name)
		}
		if err := analysis.Validate([]*analysis.Analyzer{a}); err != nil {
			t.Errorf("%s: %s", a.Name, err)
		}
	}
}

func TestRuleIDsAreRegistered(t *testing.T) {
	fromSuite := map[string]bool{}
	for _, a := range rules.Rules() {
		id := strings.TrimPrefix(a.Name, "argus")
		if id == a.Name {
			t.Errorf("%s: an analyzer name must be argus<RuleID>", a.Name)
			continue
		}
		fromSuite[id] = true
		if ergomodel.KnownRuleIDs[id] == false {
			t.Errorf("%s is in Suite but %s is not in ergomodel.KnownRuleIDs", a.Name, id)
		}
	}
	for id := range ergomodel.KnownRuleIDs {
		if fromSuite[id] == false {
			t.Errorf("%s is registered but no rule in Suite implements it", id)
		}
	}
}
