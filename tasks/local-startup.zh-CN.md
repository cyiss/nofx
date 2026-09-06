# 本机启动记录

用户要求：启动项目，Docker 相关文件放 D 盘。

- Docker Desktop 已启动，CustomWslDistroDir=D:\Docker\wsl。
- 活跃 docker_data.vhdx 位于 D:\Docker\wsl\disk，已观察到构建写入；原 C 盘大数据文件移至 D:\Docker\backups 回退副本。
- 项目完整源码运行副本在 D:\Projects\nofx-docker；源码维护位置仍是 E:\Projects\nofx。
- 私密 .env 仅当前用户、SYSTEM、Administrators 有访问权限；生成独立随机 JWT/AES/RSA，不输出密钥。
- SQLite 新数据目录已在启动前确认空；遥测关闭，传输加密开启，本机端口8080/3000。
- 独立启动检查发现 HTML 独立 GTM，不受后端遥测开关影响。已在源码和运行副本移除，并通过前端 Docker 生产构建及镜像内HTML检查。
- 首次构建因复制过滤误漏 web/src/data/faqData.ts 失败；已补齐并核对 tracked source 完整。
- 独立审阅确认新库不会恢复交易；不代注册、不访问触发自动创建钱包的 /welcome。

最终结果：两镜像本地源码构建通过，docker compose up --wait 成功，前后端均 healthy。API直连和前端代理健康接口HTTP200，浏览器登录/首次注册页正常渲染。数据库位于D盘，用户和交易员计数均0，未启动交易。
