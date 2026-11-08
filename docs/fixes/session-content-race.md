# 会话属性并发读写修复

## 问题与证据

基线：v2node `94a8381`；Xray 依赖通过 `go.mod` 固定为
`github.com/jiasu9527/xray-core v1.260327.0-jiasu3`。

该依赖的 `session.Content.SetAttribute` / `Attribute` 直接访问普通 map。
AnyTLS 在多个逻辑流中复用同一个入站 context；v2node 的 `Dispatch` 和
`DispatchLink` 原来直接使用 context 中的 `*session.Content`。多个分发因此可能
同时修改 `Protocol`、HTTP 属性，以及路由使用的 `forcedOutboundTag`。

新增测试通过实际分发入口、HTTP 嗅探和路由路径复现，替换的只是最终出站处理器，
不会访问外网。在旧代码下 `-race` 报告的栈包含：

- `Content.SetAttribute → http.SniffHTTP → Sniffer.Sniff → sniffer → Dispatch.func1`
- `Content.Attribute → GetForcedOutboundTagFromContext → routedDispatch`

这证明源码存在与报告相符的竞争，不等于已核实任何生产机器的二进制版本、重启次数，
也不证明某次网站/TLS 连接失败一定由该崩溃导致。

## 修复边界

两个分发入口均创建独立的 `Content`，并使用 `maps.Clone` 复制属性表。
调用方传入的 Content 作为只读模板；嗅探及路由的修改只作用于当前逻辑流。
空属性表、HTTP 入站预填充属性、嗅探设置、Protocol、SkipDNSResolve 和强制出口标签
均保留原有语义，强制出口标签只在子流内消费。

不关闭嗅探，不丢弃路由属性，不修改依赖缓存，也不引入全局串行锁。
此修复解决分发器共享会话内容的问题，并不使 Xray 的 Content API 本身成为
可供任意调用方并发修改的通用线程安全容器。

## 验证

```bash
GOEXPERIMENT=jsonv2 go test ./...
GOEXPERIMENT=jsonv2 go test -race ./core/app/dispatcher -count=20
GOEXPERIMENT=jsonv2 go test -race ./core/app/dispatcher -run '^TestDispatchContent' -count=100
GOEXPERIMENT=jsonv2 go vet ./core/app/dispatcher
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOEXPERIMENT=jsonv2 \
  go build -trimpath -o /tmp/v2node-linux-amd64 .
```

回归测试覆盖两个分发入口、nil/空/已有属性表、禁用嗅探时的强制出口路由、
父会话不变、嗅探后的原始请求数据保留，以及每轮每入口 64 个并发逻辑流。
发布构建新增单元测试与 dispatcher race 测试前置任务。

全项目 `go test -race ./...` 另发现 Juicity 的 `Server.Serve` / `Close` 生命周期竞争。
已在未修改的 `94a8381` 基线上复现，涉及
`TestEmbeddedJuicityStopReleasesUDPPort` 和
`TestControllerStartExternalProtocolRequestsSelfCertificate`，不属于此次 Content 修复；
因此不能将本次验证描述为“全项目 race 全绿”。

## 上线注意

这是 Go 二进制修复，仅更新管理脚本不会生效。需要构建并部署包含本修复的二进制，
或等待对应 Release 发布后再升级。当前工作流的 main 分支 push 只生成 Actions 构建产物，
不会自动替换 `releases/latest/download` 中的稳定版本文件。
