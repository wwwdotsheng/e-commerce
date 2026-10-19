# 定义数据源：告诉 Atlas 如何读取你的 Gorm 结构体
data "external_schema" "gorm" {
  program = [
    "go",
    "run",
    "-mod=mod",
    "ariga.io/atlas-provider-gorm",
    "load",
    "--path", "../internal/model",
    "--dialect", "postgres",      # 你使用的是 Postgres
  ]
}

# 迁移配置
env "gorm" {
  src = data.external_schema.gorm.url

  migration {
    dir = "file://../migrations"
  }

  # Atlas uses a disposable PostgreSQL 15 dev database to plan migrations.
  dev = "docker://postgres/15/dev"
  # Set DATABASE_URL to the target database. Never commit credentials here.
  url = getenv("DATABASE_URL")

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}