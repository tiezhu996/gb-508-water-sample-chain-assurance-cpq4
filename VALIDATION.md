# 验收记录

- 日期：2026-08-22
- 静态检查：Go 1.22.12 下 `go test ./...`、`go build ./...` 通过；Angular 类型检查和 Vite 生产构建通过；`docker compose config --quiet` 通过。
- 容器启动：MySQL、Redis、backend、frontend 均达到 healthy，`GET /healthz` 返回 200。
- API 流程：管理员登录、概览、4 个实体列表、创建采样批次、状态迁移、会话、脱敏运行配置、审计列表及审计汇总均通过。
- RBAC 与双人复核：验证 `draft` 不可跳过 `peer_review`；提交复核者不可自签；operator 不可签发；独立 reviewer 可签发并记录提交人、复核人和签发人。
- 内置 Browser：验证采样批次、实验室样本、检测方法、结果复核、审计 5 个页面；搜索、重置、新增、状态确认弹窗、`ChainBadge`、`ChainTimeline`、`MethodSelector` 和审计回显均正常；控制台 0 error / 0 warning。
- 规模：3045 行 Go 功能代码，38 个非测试 `.go` 文件。
- 清理：已执行 `docker compose down -v --remove-orphans`，无项目容器和数据卷残留。

## 2026-09-24 结果复核签发保障

- 需求：复核单由“只填关联编码”改为显式选择在检样本与在用方法；提交复核时记录样本批次、样本状态和方法版本快照；签发前再次核对，样本不再在检/删除、方法换版/退役/删除时拦截签发，被挡原因留档并写入 `block` 审核记录；被拦截复核单留档不可再提交或签发，补正后新建复核单读取最新状态。
- 后端：`ResultReview` 增加 `sampleCode/sampleBatchCode/sampleStatus/methodCode/methodVersion/blockedReason`；`Create` 强制样本 `testing`、方法 `active`；`draft -> peer_review` 时写入快照；`peer_review -> signed` 前核对实时状态，拦截返回 422 `review_blocked` 并追加审计；被拦截复核单的迁移与业务字段更新全部冻结，只能留档或由管理员删除；`go test ./...`、`go vet ./...`、`gofmt -l .` 通过。
- SQLite 端到端：11 个场景全部通过——创建预检拒绝非在检样本、提交快照（含批次与方法版本）、正常签发、样本暂停拦截与留档、拦截审计、恢复后旧单仍粘性拦截、补正新建复核单签发成功、方法换版拦截、方法退役拦截。
- 前端：结果复核页改为专用页面，新增复核弹窗只允许选择在检样本/在用方法；列表展示提交依据快照、当前状态核对、过期提示标签和被挡原因；`npm run typecheck` 与 `npm run build` 通过。
- `scripts/validate.sh` 冒烟脚本同步携带 `sampleCode/methodCode` 并断言提交时快照字段。
