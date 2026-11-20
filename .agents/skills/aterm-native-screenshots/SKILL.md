---
name: aterm-native-screenshots
description: Prepare native macOS aTerm screenshots for release notes, docs, or promotion. Use when asked to capture aTerm Home, SSH terminal, or SFTP with realistic demo data, default Light and Dark appearances, or a macOS wallpaper background.
---

# aTerm 原生窗口截图

1. 确定要展示的版本。有明确发布版本时，从对应 tag 取应用或构建。默认交付首页、SSH 终端和 SFTP 各一张浅色和深色截图。
2. 运行仓库的 `scripts/parity-fixture`，用独立的 `ATERM_DATA_DIR` 启动应用。让演示主机连接环回地址，启用真实 SSH/SFTP 协议并使用样例日志和文件。不要读取正式主机库或凭据。窗口尺寸与偏好写入演示 profile，具体做法见 [演示配置与启动](references/native-capture.md)。
3. 默认用 1780 × 1060 逻辑像素的原生窗口。先确认窗口实测尺寸，再按窗口 ID 截图。保留 macOS 窗口圆角、原生按钮和标题栏。切换页面和主题时优先使用原生窗口控制。控制不可用时，可在临时 worktree 中加仅用于打开演示页面的启动逻辑，具体做法见上述参考。
4. 默认以 Sonoma 系统壁纸铺满画布，窗口四周各留 20 个**输出像素**。保留原始截图分辨率。1800 左右的逻辑宽度在 Retina 上通常产生约 3600 像素宽的成图，以实际测量为准。
5. 逐张打开预览，确认页签与主题、窗口边缘、终端日志和 SFTP 文件可见，图中只出现演示数据。交付时说明使用的构建与数据来源。只有用户要求发布或更新文章时，才执行外部更新，并从公开页面检查图片。

## 截取与合成

从 CoreGraphics 的窗口列表按本次演示进程的 PID 找窗口编号，确认 `kCGWindowLayer` 为 0 并读取 `kCGWindowBounds`。等待页面和终端输出就绪后执行：

```sh
screencapture -x -o -l "$window_id" "$raw_png"
swift .agents/skills/aterm-native-screenshots/scripts/compose.swift \
  "$raw_png" "/System/Library/Desktop Pictures/Sonoma.heic" "$output_png"
sips -g pixelWidth -g pixelHeight "$output_png"
```

`-o` 去掉截图工具额外留下的阴影边距，合成脚本按原尺寸放置窗口。3560 × 2120 的原图得到 3600 × 2160 的成图。用 `aterm-light-home.png`、`aterm-dark-sftp.png` 一类的文件名标明主题和页面。
