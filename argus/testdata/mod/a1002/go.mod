module example.com/service

go 1.25

require (
	ergo.services/ergo v1.0.0
	example.com/library v1.2.0
)

replace (
	ergo.services/ergo => ../../src/ergo.services/ergo
	example.com/library => ./library
)
