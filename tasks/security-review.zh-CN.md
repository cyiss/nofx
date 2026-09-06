# NOFX 安全审查

审查日期：2026-09-06（Asia/Shanghai）。仓库：https://github.com/cyiss/nofx 。

本地位置：`E:\Projects\nofx`；分支：`dev`；提交：`638d4042118995fbf1a38d3822b1139aa3c6b467`。当前为浅克隆，包含该提交的完整工作树，不包含完整历史。已核对远端、提交、647 个受版本控制文件及 Git 对象连通性。

**结论：当前版本不适合未经修复直接连接真实资金或开放管理接口。** 本次审阅的安装脚本、密钥处理和资金通信路径中，没有发现足以认定其存在窃密后门、挖矿木马或隐蔽盗币代码的证据；但确认了供应链、交易风控、认证和隐私缺陷。“未发现后门”不等于安全保证。

## 审查范围和方法

- 主审加三个独立审查方向：安装/部署/依赖、认证/授权、钱包/支付/交易执行。重要发现经过第二次源码调用链复核。
- 检查了安装脚本、Docker/Compose、主要 API 路由、JWT、加密存储、钱包 onboarding、x402 付款、Hyperliquid 下单/仓位、遥测及外部请求目的地；关键词扫描用于定位，未把匹配次数当作审计覆盖率。
- 未启动后端、前端、容器、交易机器人或安装脚本；未输入任何真实密钥；未向交易所、付款网关或遥测服务发出实际业务请求。
- 对已完整阅读的 telemetry 标准库模块进行了隔离测试：原文件 SHA256 校验一致，HTTP 传输被内存替身截获，验证确实发送交易元数据并验证关闭开关有效。
- 依赖仅发送公开包名/版本到 OSV 和 npm 官方漏洞接口；没有安装项目依赖或执行 npm 生命周期脚本。
- 未运行整个项目测试、实盘/测试网测试、容器构建、govulncheck 调用图分析或性能基准；没有业务源码修改，因此不存在待验证的性能变化。以下除遥测行为测试外，均为静态证据及条件分析，不冒充已运行的攻击复现。

## 优先处理的发现

### F01 — 高：Docker 构建下载并执行未经校验的 HTTP 源码

[docker/Dockerfile.backend:22](E:/Projects/nofx/docker/Dockerfile.backend:22) 用明文 HTTP 下载 TA-Lib，随后解包，在第 32–33 行执行 `./configure` 和 `make`，没有哈希或签名验证。

构建期间能够篡改 HTTP 流量的攻击者可替换源码并执行构建阶段代码、污染最终库。多阶段构建不能消除这一输入风险，HTTP 可能重定向到 HTTPS 也不能保护第一跳。

建议：HTTPS 下载、固定版本及预先核实的 SHA256；固定基础镜像 digest；在不含运行密钥的隔离构建环境中构建。

### F02 — 高：XYZ 仓位查询失败时，风控把部分结果当作完整仓位

[trader/hyperliquid/trader_positions.go:74](E:/Projects/nofx/trader/hyperliquid/trader_positions.go:74) 在 XYZ 子查询失败时只记录警告，第 138 行仍返回 `result, nil`。[trader/auto_trader_orders.go:56](E:/Projects/nofx/trader/auto_trader_orders.go:56) 据此检查最大持仓数及同向重复持仓；空单路径同样存在。[trader/hyperliquid/trader_account.go:134](E:/Projects/nofx/trader/hyperliquid/trader_account.go:134) 的余额路径也会吞掉 XYZ 查询错误，不能依赖它一定阻止后续交易。

前提是账户实际持有 XYZ 仓位且该分区请求失败；上层可能漏算仓位并继续增加敞口，回撤监控也可能遗漏。标准 crypto 分区成功并不代表完整账户查询成功。

建议：用于交易决策的仓位快照应有完整性保证；任一必要分区失败时返回错误，禁止新增敞口，并保留明确的未知状态。独立第二审已核实该调用链。

### F03 — 高：XYZ 杠杆/保证金模式设置失败后仍可开仓

[trader/hyperliquid/trader_orders.go:64](E:/Projects/nofx/trader/hyperliquid/trader_orders.go:64) 和第 137 行对 XYZ 的 `SetLeverage` 错误只警告，随后仍调用 `placeXyzOrder`。后者未再次核验配置。[trader/hyperliquid/trader_positions.go:142](E:/Projects/nofx/trader/hyperliquid/trader_positions.go:142) 显示 `SetMarginMode` 只保存内存状态，实际杠杆及 cross/isolated 更新在 `UpdateLeverage` 中完成。

当前提为更新失败但订单成功时，交易可能沿用交易所旧杠杆/旧保证金模式，与策略配置不一致。这里不声称必然亏损或必然扩大名义头寸。限价路径也有仅警告后继续的行为。

建议：更新失败立即中止新开仓，重试前查询实际配置并核对。独立第二审已确认。

### F04 — 中：隐藏交易员仍可被匿名读取净值和余额

[api/server.go:175](E:/Projects/nofx/api/server.go:175) 将 `POST /api/equity-history-batch` 注册为公开接口。[api/handler_competition.go:395](E:/Projects/nofx/api/handler_competition.go:395) 的批量辅助函数按 ID 读取交易员、历史净值，并在第 457 行起追加实时账户信息，全程未检查 `ShowInCompetition`。对照单条接口第 138 行已有该检查。

攻击者需要知道 ID，例如该交易员过去曾公开；提交 `{"trader_ids":["已知交易员ID"]}` 即可访问隐藏后的资金历史，返回字段包括净值、余额和盈亏。公开配置接口第 494 行起也遗漏隐藏检查，泄漏名称、交易所和运行状态。这里没有证据表明接口返回私钥。

建议：批量读取和实时账户查询前，逐个验证公开权限；隐藏对象应统一拒绝公开访问。

### F05 — 中：默认遥测外发可关联的交易信息

[config/config.go:78](E:/Projects/nofx/config/config.go:78) 默认启用 `ExperienceImprovement`。[trader/auto_trader_decision.go:362](E:/Projects/nofx/trader/auto_trader_decision.go:362) 传入实际交易金额、杠杆、用户及交易员 ID；[telemetry/experience.go:117](E:/Projects/nofx/telemetry/experience.go:117) 将其与币种、交易所、交易方向、安装 ID 一并发送至 Google Analytics。AI 用量事件还包含模型、渠道和 token 用量。

隔离测试确认了交易事件目的地主机和上述字段，全部 HTTP 请求在内存中截获。这些稳定 ID 及交易数据不应被理解为完全不可关联的匿名统计。没有在该遥测载荷中发现 API secret 或钱包原始私钥。

建议：改为明确选择加入，减少发送字段。当前可设置 `EXPERIENCE_IMPROVEMENT=false` 并重启；本轮测试验证模块关闭后不再发送交易事件。

### F06 — 中：可伪造转发头绕过登录限流

[api/ratelimit.go:96](E:/Projects/nofx/api/ratelimit.go:96) 用 `c.ClientIP()` 作限流键，[api/server.go:38](E:/Projects/nofx/api/server.go:38) 使用 `gin.Default()`，未配置可信代理。锁定的 [Gin v1.11.0 源码](https://raw.githubusercontent.com/gin-gonic/gin/v1.11.0/gin.go) 默认信任所有代理，并接受转发 IP 头。

当前提为能直连后端，或反向代理没有可靠覆盖客户端提供的转发头时，更换 `X-Forwarded-For` 可获取新限流桶，削弱密码爆破及 bcrypt 资源消耗防护。该问题本身不等于免密码登录。

建议：无代理时禁用代理信任；有代理时仅信任明确地址，禁止外部直达后端并由代理覆盖转发头。

### F07 — 中：改密、找回密码不能撤销已泄漏的令牌

[api/handler_user.go:192](E:/Projects/nofx/api/handler_user.go:192) 和 [cli.go:135](E:/Projects/nofx/cli.go:135) 只更新密码；[api/server.go:667](E:/Projects/nofx/api/server.go:667) 只校验内存黑名单及 JWT，没有账号版本/密码更新时间检查。令牌默认有效期 24 小时。黑名单重启后也不保留。

前提是攻击者已取得有效令牌；受害者改密或找回后，旧令牌仍能使用，且改密接口不要求旧密码。注销过但未到期的令牌可能在重启后恢复有效。

建议：持久化会话版本或 `tokens_valid_after`，在改密/找回/删账号时更新；高风险账户操作要求重新认证。

### F08 — 中：x402 重试会为同一调用签发多份独立付款授权

[mcp/payment/x402.go:314](E:/Projects/nofx/mcp/payment/x402.go:314) 和流式路径第 449 行把再次收到 402 当作付款过期，重新签名；第 755 行每次生成随机 nonce。最多 5 次付费尝试，每份授权上限 1 USDC，没有按一次业务调用累计预算，也没有证实上一授权未结算/已失效。

前提是网关结算后仍返回 402，或网关恶意/被攻陷。它可能获得最多 5 份可独立结算的授权；余额充足且满足签名时限等条件时可能重复收费。这不是观察到 claw402.ai 实际盗扣的证据。

建议：使用幂等业务标识及累计预算；未核实前次结算状态时，不因通用 402 自动换 nonce。

## 配置与历史数据条件下的额外风险

| 风险 | 证据与成立前提 | 建议 |
|---|---|---|
| 高：孤儿钱包被新账号接管 | [handler_onboarding.go:201](E:/Projects/nofx/api/handler_onboarding.go:201) 自动收养不存在用户的 Claw402 模型，[store/ai_model.go:60](E:/Projects/nofx/store/ai_model.go:60) 无旧所有者认证，[handler_onboarding.go:91](E:/Projects/nofx/api/handler_onboarding.go:91) 响应返回原始私钥。仅当历史迁移、旧版重置或手动删除用户留下孤儿钱包时成立；当前 CLI 正常 reset 路径会删除模型，不能称其必然触发。 | 删除自动收养；钱包恢复要求原所有者证明及本地管理员操作。 |
| 高：示例 JWT secret 可通过验证 | [.env.example:23](E:/Projects/nofx/.env.example:23) 的公开占位符超过 32 字节，[config.go:96](E:/Projects/nofx/config/config.go:96) 只拒绝另一个历史默认值及短密钥。仅限手工配置未更换 JWT secret；安装脚本会生成随机值。 | 示例留空，并拒绝所有公开占位符；部署用随机密钥。 |
| 中：本地密钥文件权限过宽 | [install.sh:100](E:/Projects/nofx/install.sh:100)、[install-stable.sh:66](E:/Projects/nofx/install-stable.sh:66) 写含 JWT/AES/RSA 私钥的 `.env`，没有 umask/chmod；默认 umask 022 且父目录可遍历时其他本机用户可读。onboarding 还会把 Claw402 私钥写入 `.env`，对已存在文件的 WriteFile(0600) 不会收紧原权限。 | 生成前 umask 077；收紧现有 `.env` 权限，限制备份/访问。 |
| 中：构建上下文携带密钥和数据 | [.dockerignore](E:/Projects/nofx/.dockerignore) 未排除 `.env`、`data/`；[Dockerfile.backend:49](E:/Projects/nofx/docker/Dockerfile.backend:49) COPY 全目录。已有密钥时重建，文件进入 builder/中间层，共享或远程 builder/导出缓存可能接触。最终镜像没有直接复制这些文件，不能声称最终镜像必含密钥。 | 排除运行数据及所有密钥，仅从干净源码树构建。 |
| 中：默认全接口 HTTP 管理端口 | [docker-compose.yml:11](E:/Projects/nofx/docker-compose.yml:11)、第 45 行发布 8080/3000 未绑定主机 IP；[nginx.conf:5](E:/Projects/nofx/nginx/nginx.conf:5) 为 HTTP。实际可访问范围取决于防火墙/网络；未检测本机公网暴露。 | 默认绑定 127.0.0.1；远程管理使用 HTTPS 和隔离后端的反代。应用层公钥加密不替代 HTTPS。 |
| 条件性授权缺口 | [handler_trader_config.go:59](E:/Projects/nofx/api/handler_trader_config.go:59) 更新别人的 ID 时，数据库 0 行更新仍返回成功，随后修改全局内存可见性。需要另一个有效账号，如历史多用户库或首次注册并发竞争。类似提示词 setter 当前未发现读取方，不声称其已改变下单。 | 更新前核验所有者，检查 RowsAffected；首次注册事务化。 |

## 资金与外部通信边界

- **Hyperliquid builder 收费确实存在**：[trader/hyperliquid/trader.go:62](E:/Projects/nofx/trader/hyperliquid/trader.go:62) 的固定 builder 地址及费率会附在订单中，费率为 0.05%。前端要求钱包签名批准并显示上限；这属于明确实现的收费功能，不能据此认定为后门。
- Claw402 原始私钥用于本地 EIP-712 签名；其认证头覆盖为空，未见作为 Bearer 发送。网关收到的是钱包地址、支付签名及模型输入。签名约束 Base USDC、单份最高 1 USDC、授权最长 900 秒，但收款地址由网关指定，付款重试有 F08 风险。
- 交易所签名请求和模型/行情请求属于核心功能。交易上下文会发给选定模型服务；数据驻留不能理解为“所有账户信息都不离开本机”。Google Analytics 遥测是另外一条默认外发路径。
- 数据库中的敏感模型/交易所字段使用 AES-GCM；主程序先初始化加密服务再访问数据库。此保护不覆盖环境变量、`.env`、进程内存或合法 API 对私钥的返回，也不能抵御已控制宿主/运行进程的攻击者。
- **一键安装不等于运行本次审查的源码**：[install.sh:25](E:/Projects/nofx/install.sh:25) 指向 `NoFxAiOS/nofx/main` 并重新下载 compose；[docker-compose.prod.yml:13](E:/Projects/nofx/docker-compose.prod.yml:13) 及第 38 行运行上游 `latest` 镜像。stable 标签及 Railway 的上游二进制也不在本次源码保证范围内。应从固定提交构建或固定并核实镜像 digest。

## 依赖公告检查

Go：读取 `go.mod` 的 97 个模块版本，加上 Docker 默认 Go 1.25.11，共检查 98 项。OSV 返回 15 个去重公告：`x/crypto` 4、`x/net` 1、`x/text` 1、标准库 9。完整响应与引用见 [go-advisories.json](E:/Projects/nofx/tasks/go-advisories.json)，可用 [check-go-advisories.mjs](E:/Projects/nofx/tasks/check-go-advisories.mjs) 重查。

这属于版本匹配，**不是 15 个已证实可远程利用的项目漏洞**。例如 `x/crypto` 公告涉及 SSH/OpenPGP，本轮未在项目自有源码看到这些包的直接使用，未完成传递依赖调用图。Go 1.25.11 已低于多份公告修复版本，例如 [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) 和 [GO-2026-6088](https://pkg.go.dev/vuln/GO-2026-6088)；发布前应使用包含修复的受支持工具链并执行 govulncheck。实际预构建二进制的 Go 版本未验证。

npm：官方 bulk advisories 返回 7 个受影响包、9 个去重 GHSA。6 个包在锁文件中仅作为开发依赖；生产依赖 `react-router 7.18.1` 命中的 [GHSA-qwww-vcr4-c8h2](https://github.com/advisories/GHSA-qwww-vcr4-c8h2) 限定 RSC 模式，本项目使用 BrowserRouter 静态 SPA，未确认适用。详情见 [npm-bulk-advisories.json](E:/Projects/nofx/tasks/npm-bulk-advisories.json)。开发依赖风险应更新处理，但不直接映射为生产 API 漏洞。

`npm audit --package-lock-only --ignore-scripts --json` 长时间无输出，已终止，未宣称该 CLI 扫描通过。上述 npm 结论来自直接官方 bulk API 的独立检查。

## 验证记录与交付边界

- Git：origin 为指定仓库；当前分支 dev；HEAD 如上；`git fsck --connectivity-only` 成功；`git diff --exit-code HEAD` 无业务文件变更。
- 遥测：隔离 Go 测试 `TestTradeTelemetrySendsFinancialMetadata` 通过；原源码与复制件 SHA256 均为 `B6BC07B477DA71E4E9E40A337D4F27857AE4D1B5368A732D46A722E45BC46AF4`。测试位于 [tasks/verification/telemetry](E:/Projects/nofx/tasks/verification/telemetry)。测试证实外发行为及关闭开关，不是整个项目安全测试通过。
- 两个 XYZ 交易风险由另一审查代理独立复核；认证、部署及资金支付发现由主审再次阅读相关源码。未向任何实际服务发送漏洞探测请求。
- 本轮只添加 `tasks/` 下的审查计划、报告、证据和隔离测试，未改业务源码、安装/启动项目、提交或推送。
- 未涵盖完整 Git 历史、所有依赖实现、上游镜像/二进制供应链、宿主安全、交易所侧权限或全部交易失败场景。未确认可利用的 SQL 注入、路径穿越或前端 XSS，但这不代表已证明不存在。

建议先修复 F01–F03，再处理认证、数据权限及付款幂等；在隔离环境以虚构凭据/mock 和故障注入验证之后，再考虑接入仅有必要权限的交易账户。当前审查没有授权或触发真实交易。
