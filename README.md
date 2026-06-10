# v2node
A v2board backend base on moddified xray-core.
一个基于修改版xray内核的V2board节点服务端。

**注意： 本项目需要搭配[修改版V2board](https://github.com/wyx2685/v2board)**

## 软件安装

### 一键安装

```
curl -fsSL -H 'Cache-Control: no-cache' "https://raw.githubusercontent.com/jiasu9527/v2node/main/script/install.sh?ts=$(date +%s)" -o install.sh && bash install.sh
```

安装完成后可在首次安装提示里分别配置 Cloudflare DDNS 和被墙检测/自动换 IP，也可以后续执行：

```bash
v2node ddns          # 配置 Cloudflare DDNS
v2node block-check   # 配置被墙检测/自动换 IP
v2node ddns-status   # 查看状态
v2node ddns-run      # 立即执行一次 DDNS/墙检测
v2node block-check-run # 只执行一次被墙检测/自动换 IP
v2node ddns-disable  # 停用定时任务
```

DDNS 仅内置 Cloudflare；墙检测默认访问 `https://baidu.com/`，检测失败即计为异常；也可自定义检测接口，支持 `{ip}` / `{domain}` 占位符。
默认每 1 分钟检查一次；默认 1 次异常就执行换 IP curl，换 IP 等待和冷却均为 0，只有 IP 变化时才更新 Cloudflare 解析。

非交互安装可追加 DDNS 参数：

```bash
bash install.sh --api-host https://example.com --node-id 1 --api-key key \
  --enable-ddns --cf-token CF_API_TOKEN --cf-zone-id CF_ZONE_ID --cf-record node.example.com
```

## 构建
``` bash
version=v0.3.9
GOEXPERIMENT=jsonv2 go build -v -o build_assets/v2node -trimpath -ldflags "-X 'github.com/wyx2685/v2node/cmd.version=$version' -s -w -buildid="
```

## Stars 增长记录

[![Stargazers over time](https://starchart.cc/wyx2685/v2node.svg?variant=adaptive)](https://starchart.cc/wyx2685/v2node)
