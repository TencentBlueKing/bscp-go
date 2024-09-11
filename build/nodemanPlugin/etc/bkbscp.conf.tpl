# 插件全局配置
path.pid: {{ plugin_path.pid_path }}
path.logs: {{ plugin_path.log_path }}
path.data: {{ plugin_path.data_path }}
host_id_path: {{ plugin_path.host_id }}
{%- if nodeman is defined %}
hostip: {{ nodeman.host.inner_ip }}
{%- else %}
hostip: {{ cmdb_instance.host.bk_host_innerip_v6 if cmdb_instance.host.bk_host_innerip_v6 and not cmdb_instance.host.bk_host_innerip else cmdb_instance.host.bk_host_innerip }}
{%- endif %}
cloudid: {{ cmdb_instance.host.bk_cloud_id[0].id if cmdb_instance.host.bk_cloud_id is iterable and cmdb_instance.host.bk_cloud_id is not string else cmdb_instance.host.bk_cloud_id }}
hostid: {{ cmdb_instance.host.bk_host_id }}
bk_agent_id: '{{ cmdb_instance.host.bk_agent_id }}'

# 业务id
biz: {{ biz }}
# 服务配置
apps:
  {%- for app in apps %}
  - name: {{ app.name }}
    labels:
      {%- for label in app.labels %}
      {{ label.key }}: "{{ label.value }}"
      {%- endfor %}
    config_matches:
      {%- for match in app.config_matches %}
      - "{{ match }}"
      {%- endfor %}
  {%- endfor %}

# 客户端密钥
token: {{ token }}
# 临时目录
temp_dir: {{ temp_dir }}
{%- if 全局标签 %}
# 全局标签
labels:
  {%- for label in global_labels %}
  {{ label.key }}: "{{ label.value }}"
  {%- endfor %}
{%- endif %}
{%- if global_config_matches %}
# 全局配置文件拉取筛选
config_matches:
  {%- for match in global_config_matches %}
  - "{{ match }}"
  {%- endfor %}
{%- endif %}

# Feed服务地址
feed_addrs:
  - {{ feed_addr }}

# 是否启用P2P文件下载加速
{%- if enable_p2p_download %}
enable_p2p_download: true
{%- else %}
enable_p2p_download: false
{%- endif %}

# 是否上报资源
{% if enable_resource %}
enable_resource: true
{% else %}
enable_resource: false
{% endif %}

# 文本文件换行符
text_line_break: {{ text_line_break }}