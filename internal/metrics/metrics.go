// Package metrics 提供进程级 expvar 指标（标准库，供各层共享计数）。
// 该包是叶子包：只依赖标准库，可被 domain 与 adapter 安全 import。
package metrics

import "expvar"

// 进程级计数器（幂等注册）。
var (
	ChainDetected  = expvar.NewInt("launch_chain_detected_total")
	SocialDetected = expvar.NewInt("launch_social_detected_total")
	DecodeErrors   = expvar.NewInt("launch_decode_errors_total")
	VerifyPass     = expvar.NewInt("launch_verify_pass_total")
	VerifyFail     = expvar.NewInt("launch_verify_fail_total")
	Dropped        = expvar.NewInt("launch_dropped_total")
	WSReconnects   = expvar.NewInt("launch_ws_reconnects_total")
)
