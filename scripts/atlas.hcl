# 定义数据源：告诉 Atlas 如何读取你的 Gorm 结构体
data "external_schema" "gorm" {
  program = [
    "go",
    "run",
    "-mod=mod",
    "ariga.io/atlas-provider-gorm",
    "load",
    "--path", "../internal/user", # 替换为你存放实体类（Struct）的路径
    "--dialect", "postgres",      # 你使用的是 Postgres
  ]
}

# 迁移配置
env "gorm" {
  src = data.external_schema.gorm.url

  migration {
    dir = "file://../migrations"
  }

  # 修改这里：指向你 docker-compose 里的 5432 端口
  # 注意：必须加上 sslmode=disable，因为本地容器通常没配 SSL
  dev = "postgres://postgres:postgres@localhost:5432/dev?sslmode=disable"
  url = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}