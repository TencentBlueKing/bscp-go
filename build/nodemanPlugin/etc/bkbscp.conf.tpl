# 插件全局配置
path.pid: "{{ plugin_path.pid_path }}"
path.logs: "{{ plugin_path.log_path }}"
path.data: "{{ plugin_path.data_path }}"
host_id_path: "{{ plugin_path.host_id }}"
{%- if nodeman is defined %}
hostip: {{ nodeman.host.inner_ip }}
{%- else %}
hostip: {{ cmdb_instance.host.bk_host_innerip_v6 if cmdb_instance.host.bk_host_innerip_v6 and not cmdb_instance.host.bk_host_innerip else cmdb_instance.host.bk_host_innerip }}
{%- endif %}
cloudid: {{ cmdb_instance.host.bk_cloud_id[0].id if cmdb_instance.host.bk_cloud_id is iterable and cmdb_instance.host.bk_cloud_id is not string else cmdb_instance.host.bk_cloud_id }}
hostid: {{ cmdb_instance.host.bk_host_id }}
bk_agent_id: "{{ cmdb_instance.host.bk_agent_id }}"

# 业务id
biz: {{ 业务ID }}
apps: {% for app in 服务 %}
  - name: {{ app.服务名称 }}
    labels: {% for label in app.标签 %}
        {{ label.key }}: {{ label.value }}{% endfor %}
    {% endfor %}
# token
token: {{ 客户端密钥 }}
temp_dir: {{ 临时目录 }}
{% if 全局标签 %}labels:
{% for label in 全局标签 %}{{ "-" if loop.first else " "  }} {{ label.key }}: "{{ label.value }}"
{% endfor %}
{% endif %}

feed_addrs:
  - {{ Feed服务地址 }}

# 是否启用P2P文件下载加速
{% if P2P文件下载加速 %}
enable_p2p_download: true
{% else %}
enable_p2p_download: false
{% endif %}
