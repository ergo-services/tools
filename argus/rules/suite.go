package rules

import (
	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
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

func Suite() []*analysis.Analyzer {
	return []*analysis.Analyzer{

		ergomodel.Analyzer,

		messagealiasing.Analyzer,
		callbackblocking.Analyzer,
		selfrequest.Analyzer,
		callbackgoroutine.Analyzer,
		metastateshare.Analyzer,
		ownstatesend.Analyzer,
		internalescape.Analyzer,
		handlecallreason.Analyzer,
		goroutinerecover.Analyzer,
		nodesend.Analyzer,
		initbudget.Analyzer,
		stategate.Analyzer,
		uncheckedresult.Analyzer,
		noreply.Analyzer,
		doublereply.Analyzer,
		wireshape.Analyzer,
		supervisorspec.Analyzer,
		timermisuse.Analyzer,
		eventprotocol.Analyzer,
		webrequest.Analyzer,
		registration.Analyzer,
		spawnargs.Analyzer,
		logreturn.Analyzer,
		eventbuffer.Analyzer,
		terminateio.Analyzer,
		terminatehalfbuilt.Analyzer,
		timerchain.Analyzer,
		deferredidentity.Analyzer,
		initsentinel.Analyzer,
		eventnotify.Analyzer,
		wireerror.Analyzer,
		wiretypeclosure.Analyzer,
		wiresentinel.Analyzer,
		appinitleak.Analyzer,
		callbackroundtrip.Analyzer,
		behaviorspec.Analyzer,
		alwaysfails.Analyzer,
		callbudget.Analyzer,
		metastart.Analyzer,
		messagemarker.Analyzer,
		registrylist.Analyzer,
		suppressiondebt.Analyzer,
		intentcomment.Analyzer,
		housethreshold.Analyzer,
	}
}

func Rules() []*analysis.Analyzer {
	var out []*analysis.Analyzer
	for _, a := range Suite() {
		if a == ergomodel.Analyzer {
			continue
		}
		out = append(out, a)
	}
	return out
}
