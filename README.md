# v2node
A v2board backend base on moddified xray-core.
一个基于修改版xray内核的V2board节点服务端。

**注意： 本项目需要搭配[修改版V2board](https://github.com/wyx2685/v2board)**

## 软件安装

### 一键安装

```
curl -fsSL -H 'Cache-Control: no-cache' "https://raw.githubusercontent.com/jiasu9527/v2node/main/script/install.sh?ts=$(date +%s)" -o install.sh && bash install.sh
```

安装完成后可在首次安装提示里配置 Cloudflare DDNS/墙检测自动换 IP，也可以后续执行：

```bash
v2node ddns
v2node ddns-status
v2node ddns-run
v2node ddns-disable
```

DDNS 仅内置 Cloudflare；墙检测接口和换 IP API 使用安装时输入的 curl 命令，检测接口支持 `{ip}` / `{domain}` 占位符。

## 构建
``` bash
version=v0.3.9
GOEXPERIMENT=jsonv2 go build -v -o build_assets/v2node -trimpath -ldflags "-X 'github.com/wyx2685/v2node/cmd.version=$version' -s -w -buildid="
```

## Stars 增长记录

[![Stargazers over time](https://starchart.cc/wyx2685/v2node.svg?variant=adaptive)](https://starchart.cc/wyx2685/v2node)
