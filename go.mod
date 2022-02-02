module sing

go 1.18

require (
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da
	github.com/dgryski/go-camellia v0.0.0-20191119043421-69a8a13fb23d
	github.com/dgryski/go-idea v0.0.0-20170306091226-d2fb45a411fb
	github.com/dgryski/go-rc2 v0.0.0-20150621095337-8a9021637152
	github.com/geeksbaek/seed v0.0.0-20180909040025-2a7f5fb92e22
	github.com/kierdavis/cfb8 v0.0.0-20180105024805-3a17c36ee2f8
	golang.org/x/crypto v0.0.0-20220126234351-aa10faf2a1f8
)

// for testing and example only

require (
	github.com/Dreamacro/clash v1.9.0
	github.com/v2fly/v2ray-core/v5 v5.0.3
)

//replace github.com/v2fly/v2ray-core/v5 => ../v2ray-core
replace github.com/v2fly/v2ray-core/v5 => github.com/sagernet/v2ray-core/v5 v5.0.7-0.20220128184540-38f59e02f567

// https://github.com/google/gvisor/releases/tag/release-20211129.0
//replace gvisor.dev/gvisor => ../gvisor
replace gvisor.dev/gvisor => github.com/sagernet/gvisor v0.0.0-20220109124627-f8f67dadd776

require (
	github.com/Dreamacro/go-shadowsocks2 v0.1.7 // indirect
	github.com/dgryski/go-metro v0.0.0-20211217172704-adc40b04c140 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/pires/go-proxyproto v0.6.1 // indirect
	github.com/riobard/go-bloom v0.0.0-20200614022211-cdc8013cb5b3 // indirect
	github.com/seiflotfy/cuckoofilter v0.0.0-20201222105146-bc6005554a0c // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/v2fly/ss-bloomring v0.0.0-20210312155135-28617310f63e // indirect
	golang.org/x/mod v0.6.0-dev.0.20211013180041-c96bc1413d57 // indirect
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)
