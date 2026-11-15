# Mac App Store 签名配置

签名材料存放在仓库外的 `~/.config/aterm/signing/`，不提交到 Git：

```text
~/.config/aterm/signing/
├── SSH_Config_Manager_MAS.key
├── distribution.cer
├── mac_installer.cer
├── embedded.provisionprofile
└── AppleWWDRCAG3.cer
```

私钥必须与签名证书匹配。CSR（`*.certSigningRequest`）仅在申请证书时使用，可一并保存在这个目录，不参与打包。Apple WWDR 中间证书不是秘密，缺失时脚本会尝试从 Apple 下载。

将目录权限设为 `700`，私钥文件权限设为 `600`。另外安全备份签名材料。

脚本仍会通过 macOS Keychain 完成签名：读取本地材料、按需导入签名身份，然后调用签名工具。临时 `.p12` 文件默认使用随机密码，并在脚本退出时删除；导入的身份会保留在 Keychain 中。

默认直接运行原有打包命令即可。自定义整个材料目录：

```bash
SIGNING_DIR="/path/to/private/signing" wails3 task mas:upload
```

也可以单独覆盖 `SIGNING_KEY`、`PROVISIONING_PROFILE`、`WWDR_CERT` 和 `KEYCHAIN`。`SIGNING_DIR` 中的 `.cer` / `.cert` 文件会被自动导入；`APPSTORE_IDENTITY` 和 `INSTALLER_IDENTITY` 可指定已有签名身份。脚本默认使用 `distribution.cer` 和 `mac_installer.cer` 的 SHA-1 指纹选择签名身份，并限定使用指定的 Keychain，避免同名证书造成歧义。环境变量也支持指定证书 SHA-1 指纹；指定名称时必须唯一匹配。不要把私钥内容或密码写入脚本或提交到仓库。

从公开仓库克隆的开发者需要自行准备正式发布签名材料；普通开发运行不需要发布者的私钥。

## 打包

在仓库根目录执行：

```bash
wails3 task mas:upload
```

生成的上传包是：

```text
build/bin/aTerm.pkg
```

Transporter 上传这个 `.pkg` 文件，不是上传 `.app`。

## 版本号和 Build 号

版本号配置在 `build/config.yml`：

```yaml
info:
  productName: "aTerm"
  productIdentifier: "pro.huabei.sshconfig"
  version: "2.3.0"
  buildNumber: "55"
```

- `info.version` 会写入 `CFBundleShortVersionString`，也就是 App Store Connect 里看到的版本号。
- `info.buildNumber` 会写入 `CFBundleVersion`，也就是 App Store Connect 里的 build 号。

每次执行 `wails3 task mas:package`、`wails3 task mas:upload` 或 `wails3 task mas:upload:cli` 时，脚本会自动把 `version` 的 patch 位和 `buildNumber` 都加 1。例如 `2.3.0` 会变成 `2.3.1`，`x.y` 不会自动变化。

当前 Mac App Store 包是 arm64-only 构建。App Store Connect 对 arm64-only App 的最低要求是 macOS 12.0。

上传前也可以单独验证签名和架构元数据：

```bash
wails3 task mas:validate
```
