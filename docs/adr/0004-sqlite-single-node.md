# 生产环境用 SQLite 单文件数据库，不装 Redis

上游 docker-compose 默认用 PostgreSQL + Redis，这里刻意不用。原因是成员只有站长的熟人、访问量很小，服务器只有 2 核 2 GB：SQLite 单文件省内存，备份就是复制文件。

代价是只能单机运行。新增的表和查询仍按上游规则保持 SQLite / MySQL / PostgreSQL 三库兼容，并在 CI 里用 MySQL、PostgreSQL 服务容器验证，以便将来迁移。
