# AnyTLS 生命周期与内存滞留修复

## 已验证的问题

基线 Xray `github.com/jiasu9527/xray-core v1.260327.0-jiasu3`，v2node
`1745038` 与 `c521a92` 均包含这些生命周期缺陷；`c521a92` 的会话属性
隔离修复针对 concurrent map 崩溃，并非本次内存问题的修复。

- AnyTLS UoT encode/decode writer 没有转发 Close/Interrupt，底层 pipe 的
  EOF/中断被丢弃，导致 DNS/UDP 出站、下行泵及用户连接记录滞留。
- AnyTLS Server.Process 返回未调用 session.close；pipe.Reader 仅实现
  Interrupt，而 stream.close 使用 common.Close，无法唤醒下行泵。
- AnyTLS Server 没有 Close，节点重载不会停止用户更新 goroutine，旧用户
  快照仍被引用。
- LinkManager.CloseAll 持锁调用 ManagedWriter.Close，后者通过 RemoveWriter
  再次获取相同锁，非空连接集合会死锁，用户同步可能因此超时。

生产现场曾有 RSS 约 3.3 GiB、进程 Swap 约 3.9 GiB 和内核 OOM 记录。
重启后的 11 分钟采样中，AnyTLS 物理读循环由 290 变为 285，下行泵由
255 增至 981，与资源清理失效吻合。由于内存耗尽瞬间未启用 pprof，不能
把全部占用定量归到某一个对象；更不能把所有 Go RSS 上涨都判为泄漏。

## 修复范围

1. UoT 包装器透传 Close/Interrupt；普通 Close 保留缓冲包的排空语义，
   Interrupt 可唤醒背压阻塞。不在关闭线程无锁修改 pending 缓冲区。
2. ManagedWriter 增加 Interrupt；LinkManager 锁内摘取连接、锁外关闭，
   已关闭的 manager 拒绝迟到的连接；两个 dispatcher 入口用 LoadOrStore
   保证并发首次建连不会覆盖另一个 manager。
3. Server 跟踪会话并在关闭时禁止新会话、取消 context、关闭物理连接、
   释放流，等待会话与用户更新协程退出，清空旧快照/待更新集合。
4. Process 的所有退出路径都清理会话并等待下行泵退出；取消 context 也可
   打断物理读写。会话异常关闭中断上下行，普通流 FIN 则仍允许 uplink
   已缓冲数据排空，并中断 downlink Reader。
5. link 发布与流关闭同步，避免关闭期间 Dispatch 晚返回留下孤儿 pipe；
   stream.close 幂等。下行泵按对象身份删除流，避免误删复用同 ID 的流。
6. Dispatch 失败及时释放输入并发送 FIN，不再留下空流；补齐地址解析、
   UDP 请求解析和 SYNACK 失败路径的 MultiBuffer 释放。
7. 修复 sendStreamData 的 padding 写失败路径未解锁的问题，避免后续清理
   被同一写锁卡住。

依赖源码固定在 `third_party/xray-core`，原版本逐文件哈希见
`third_party/xray-core.base-sha256.json`，保留 MPL-2.0 许可证。只修改
AnyTLS 的四个实现文件并新增生命周期测试，不更新其他 Xray 协议版本。
详见 `third_party/README.md`。不是修改全局模块缓存或只改本机可用的构建。

## 回归验证

```sh
GOEXPERIMENT=jsonv2 go test ./...
GOEXPERIMENT=jsonv2 go test -race ./core/app/dispatcher github.com/xtls/xray-core/proxy/anytls -count=100 -timeout=180s
GOEXPERIMENT=jsonv2 go vet ./core/app/dispatcher github.com/xtls/xray-core/proxy/anytls
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOEXPERIMENT=jsonv2 \
  go build -trimpath -ldflags '-X github.com/wyx2685/v2node/cmd.version=v0.3.9-memfix.20261117.1' -o output/v2node-linux-amd64 .
```

覆盖真实 AnyTLS auth/settings/SYN/PSH/SYNACK/FIN 帧与真实 Xray pipe：
TCP/UDP 双向数据、客户端断连、单流 FIN、context 取消、Server.Close、
握手中关闭、背压中取消、晚返回 Dispatch、用户更新并发关闭、失败流
清理，以及 30 次建连断连后的会话清空。UoT 回归同时验证关闭、中断、
背压解除和关闭前已缓存数据的交付。

测试使用内存 dispatcher，不访问公网。现有 Content 隔离回归仍保留。
`go test ./...` 不会发现嵌套模块的测试，发布 CI 已显式执行 AnyTLS 包。
全项目普通单测、以上指定包的 100 轮 race 和 vet 均通过；不宣称全项目
race 通过（已知 Juicity 基线竞争见 session-content-race.md）。

## 部署与观察

首台灰度版本标识：`v0.3.9-memfix.20261117.1`。保留现有节点配置、路由
和仅回环地址可访问的 pprof。部署需备份旧二进制/配置，核验上传 SHA-256，
原子替换后检查两个节点监听、pprof、版本及 PID；失败回滚。

此版本包含已有 Content 修复；没有添加定时重启、硬内存上限或自动升级
其他服务器。观察时同时对比连接数、live heap、RSS 与 AnyTLS 下行泵：
正常 Go 堆预留、缓冲池和业务负载会让 RSS 波动，不能只凭 RSS 不下降
认定修复失败。短时回归通过不等同于已完成数天的生产稳定性验证。
