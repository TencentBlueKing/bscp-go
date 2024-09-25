name: bkbscp
version: "__VERSION__"
description: bscp服务配置分发和热更新
scenario: bscp服务配置分发和热更新
category: official
config_file: bkbscp.conf
config_format: yaml
auto_launch: false
launch_node: all
is_binary: true
use_db: false
config_templates:
  - plugin_version: "__VERSION__"
    name: bkbscp.conf
    version: "__VERSION__"
    file_path: etc
    format: yaml
    is_main_config: 1
    source_path: etc/bkbscp.conf.tpl
    variables:
      title: variables
      type: object
      required: true
      properties:
        biz:
          title: 业务ID
          type: string
          required: true
        apps:
          title: 服务
          type: array
          required: true
          items:
            title: 服务配置
            type: object
            required: true
            properties:
              name:
                title: 服务名称
                type: string
                required: true
              labels:
                title: 标签
                type: array
                required: false
                items:
                  title: label
                  type: object
                  required: false
                  properties:
                    key:
                      title: key
                      type: string
                      required: true
                    value:
                      title: value
                      type: string
                      required: true
              config_matches:
                title: 配置文件筛选
                type: array
                required: false
                items:
                  title: 匹配规则
                  type: string
                  required: true
        feed_addr:
          title: 服务feed-server地址
          type: string
          required: true
        token:
          title: 客户端密钥
          type: string
          required: true
        global_labels:
          title: 全局标签
          type: array
          required: false
          items:
            title: label
            type: object
            required: false
            properties:
              key:
                title: key
                type: string
                required: true
              value:
                title: value
                type: string
                required: true
        global_config_matches:
          title: 全局配置文件筛选
          type: array
          required: false
          items:
            title: 匹配规则
            type: string
            required: true
        temp_dir:
          title: 临时目录
          type: string
          required: true
          default: /data/bscp
        enable_p2p_download:
          title: P2P文件下载加速
          type: boolean
          required: true
          default: false
        enable_resource:
          title: 是否上报资源
          type:  boolean
          required: true
          default: true
        text_line_break:
          title: 文本文件换行符
          type: string
          required: false
          default: LF
control:
  start: "__START_SCRIPT__ bkbscp"
  stop: "__STOP_SCRIPT__ bkbscp"
  restart: "__RESTART_SCRIPT__ bkbscp"
  reload: "__RELOAD_SCRIPT__ bkbscp"
  version: "./bkbscp -v"
