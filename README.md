# V2bX

[![](https://img.shields.io/badge/TgChat-UnOfficialV2Board%E4%BA%A4%E6%B5%81%E7%BE%A4-green)](https://t.me/unofficialV2board)
[![](https://img.shields.io/badge/TgChat-YuzukiProjects%E4%BA%A4%E6%B5%81%E7%BE%A4-blue)](https://t.me/YuzukiProjects)

A V2board node server based on multi core, modified from XrayR.  
一个基于多种内核的V2board节点服务端，修改自XrayR，支持V2ay,Trojan,Shadowsocks协议。

**注意： 本项目需要搭配[修改版V2board](https://github.com/wyx2685/v2board)**

## 本分支

在 [wyx2685/V2bX](https://github.com/wyx2685/V2bX) 之上，原有内核和功能不变，另外：

* **模块**：额外功能放在 `module/` 下，各自一个编译标签，不带标签编译时一行代码都不进程序（见 `module/module.go`）。
  删一个模块 = 删掉它的目录和 `module/imports/` 里对应的一个文件。
* **链式代理**（`chain` 模块）：面板把节点设成"入口"后，它的连接依次经过面板指定的其它节点再出去，
  互联网看到的是最后一个节点的地址。只有入口节点需要本程序，经过的节点照常对接即可。
  出错时一律拒绝连接，绝不从入口直接出去。
* **在线更新**（`upgrade` 模块）：管理员在面板上点「更新」，节点才从本仓库的 Release 下载新版本（不会自己更新），
  校验 SHA-256、试运行后原地替换并重启；新版本起不来就换回旧版本并告诉面板原因。不留下任何旧文件：
  下载的文件在重启前删掉，旧程序在新程序跑起来后删掉。只认本仓库的发布（或 `V2BX_RELEASES` 指向的镜像），
  面板只能选版本，不能让节点去下载别的程序。
* 每次请求面板都带上程序版本、内核和已编入的模块（`X-V2bX-*` 请求头），面板据此只要求节点做它能做的事。
* 多内核时自动选内核的结果固定（原来每次启动可能不同）。

## 特点

* 永久开源且免费。
* 支持Vmess/Vless, Trojan， Shadowsocks, Hysteria1/2多种协议。
* 支持Vless和XTLS等新特性。
* 支持单实例对接多节点，无需重复启动。
* 支持限制在线IP。
* 支持限制Tcp连接数。
* 支持节点端口级别、用户级别限速。
* 配置简单明了。
* 修改配置自动重启实例。
* 支持多种内核，易扩展。
* 支持条件编译，可仅编译需要的内核。

## 功能介绍

| 功能        | v2ray | trojan | shadowsocks | hysteria1/2 |
|-----------|-------|--------|-------------|----------|
| 自动申请tls证书 | √     | √      | √           | √        |
| 自动续签tls证书 | √     | √      | √           | √        |
| 在线人数统计    | √     | √      | √           | √        |
| 审计规则      | √     | √      | √           | √         |
| 自定义DNS    | √     | √      | √           | √        |
| 在线IP数限制   | √     | √      | √           | √        |
| 连接数限制     | √     | √      | √           | √         |
| 跨节点IP数限制  |√      |√       |√            |√          |
| 按照用户限速    | √     | √      | √           | √         |
| 动态限速(未测试) | √     | √      | √           | √         |

## TODO

- [ ] 重新实现动态限速
- [ ] 完善使用文档

## 软件安装

### 一键安装 / 更新 / 替换官方版

```
bash <(curl -Ls https://raw.githubusercontent.com/DesuwaDev/V2bX/dev_new/script/install.sh)
```

装在官方 V2bX（V2bX-script）的同一位置，`/etc/V2bX` 的配置原样保留，之后 `V2bX update` 也从本仓库更新。
新程序先下载到临时目录、校验 SHA-256、试运行，才替换；新程序没能稳定运行时自动换回原程序，
稳定运行后原程序删掉，不留备份。

### 手动安装

[手动安装教程](https://v2bx.v-50.me/v2bx/v2bx-xia-zai-he-an-zhuang/install/manual)

## 构建
``` bash
# 通过-tags选项指定要编译的内核， 可选 xray， sing, hysteria2；以及模块，可选 chain, upgrade
GOEXPERIMENT=jsonv2 go build -v -o build_assets/V2bX -tags "sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor chain upgrade" -trimpath -ldflags "-X 'github.com/InazumaV/V2bX/cmd.version=$version' -s -w -buildid="
```

## 配置文件及详细使用教程

[详细使用教程](https://v2bx.v-50.me/)

## 免责声明

* 此项目用于本人自用，因此本人不能保证向后兼容性。
* 由于本人能力有限，不能保证所有功能的可用性，如果出现问题请在Issues反馈。
* 本人不对任何人使用本项目造成的任何后果承担责任。
* 本人比较多变，因此本项目可能会随想法或思路的变动随性更改项目结构或大规模重构代码，若不能接受请勿使用。

## 赞助

[赞助链接](https://v-50.me/)

## Thanks

* [Project X](https://github.com/XTLS/)
* [V2Fly](https://github.com/v2fly)
* [VNet-V2ray](https://github.com/ProxyPanel/VNet-V2ray)
* [Air-Universe](https://github.com/crossfw/Air-Universe)
* [XrayR](https://github.com/XrayR/XrayR)
* [sing-box](https://github.com/SagerNet/sing-box)

## Stars 增长记录

[![Stargazers over time](https://starchart.cc/wyx2685/V2bX.svg)](https://starchart.cc/wyx2685/V2bX)
