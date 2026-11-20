# 演示配置与启动

## 准备独立的本地 SSH/SFTP 环境

从仓库根目录运行以下命令。保持 fixture 进程运行，待它打印 `Fixture ready` 后，才复制或修改演示 profile。

```sh
capture_root="$(mktemp -d -t aterm-native-capture)"
python3 scripts/go-macos.py run ./scripts/parity-fixture \
  -cwd /home/deploy -profile "$capture_root/fixture"
```

默认 fixture 的内容适合连通性测试。需要宣传截图时，可在临时 worktree 修改 fixture 的 `seed` 和 `shell` 分支。先创建 SFTP 目录 `/home/deploy`、`config`、`logs` 和 `releases`，再填入 `nginx.conf`、`README.md` 等样例文件。构造真实的命令行外观，例如 Nginx 状态、配置检查与访问日志，作为 fixture 的 shell 输出。用 `sftp.WithStartDirectory("/home/deploy")` 对齐 SFTP 初始路径。保持现有环回限制，别让它执行真实主机上的命令。

按场景复制已初始化的 profile。在副本的 `app.db` 中：

- 把 fixture 创建的主机改名为 `web-staging`，保留它的环回端口和密钥。可加预发布、生产和基础设施等目录，固定几个演示主机，填入看得懂的名称与备注。
- 将 `app_metadata` 的 `desktop-main-window-v1` 写为例如 `{"x":86,"y":60,"width":1780,"height":1060,"maximized":false}`。在启动前写入，应用会按屏幕工作区调整尺寸。
- 将 `desktop-preferences-v1` 写为 `{"language":"zh-CN","theme":"light"}` 或 `{"language":"zh-CN","theme":"dark"}`。默认主题下保留 `lightTheme` 和 `darkTheme` 的默认值。若目标版本的元数据字段不同，查阅该版本的 `internal/app/window_state.go` 与 `internal/desktop/preferences.go`。

```sh
open -n --env "ATERM_DATA_DIR=$profile" "$demo_app"
```

通过系统窗口列表确认选中的是这次启动的进程，避免截到同时运行的日常 aTerm。场景之间可分别启动演示实例，清理时只结束自己启动的进程。

## 原生窗口控制不可用时

在目标版本的临时 Git worktree 中构建演示版。生产分支不需要这些改动。

1. 在 `internal/app/main_window.go` 给原生窗口的初始 URL 增加仅由演示环境变量触发的参数，例如 `/?capture=terminal` 或 `/?capture=sftp`。首页无参数。
2. 在 `frontend/src/renderer/src/app/App.tsx` 的 Workbench 中读取参数。等 `library.hosts` 加载且 `web-staging` 可用后，用 `useRef` 保证只执行一次，调用现有的 `connectHost(host.id)` 或 `openFiles(host.id)`。用真实的工作区和后端连接逻辑，避免替换 React 组件、IPC 或窗口边框。
3. 运行该版本的 macOS 构建流程，按场景启动它。先构建前端再编译嵌入资源。如果复用对应版本的 app bundle 并替换二进制，只操作临时副本，替换后重新签名。
4. 实测终端已经显示样例 shell 输出且 SFTP 已进入预期目录，再截取系统窗口。若无法获得原生画面，说明缺少的场景。交付时标明用了临时构建及其启动差异。
