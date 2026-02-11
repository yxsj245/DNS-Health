/**
 * 云服务商凭证字段配置注册表
 *
 * 每个 provider 定义自己需要的凭证字段，前端表单根据此配置动态渲染。
 * 新增服务商时只需在此文件添加配置即可，无需修改表单组件。
 *
 * 字段说明：
 * - key: 字段标识，对应后端 credentials map 的 key
 * - label: 表单标签（中文）
 * - placeholder: 输入提示
 * - type: 输入类型，"text" 或 "password"
 * - required: 是否必填
 */
const providerConfig = {
  aliyun: {
    label: '阿里云',
    fields: [
      {
        key: 'access_key_id',
        label: 'AccessKey ID',
        placeholder: '请输入 AccessKey ID',
        type: 'text',
        required: true
      },
      {
        key: 'access_key_secret',
        label: 'AccessKey Secret',
        placeholder: '请输入 AccessKey Secret',
        type: 'password',
        required: true
      }
    ]
  },
  // Cloudflare DNS 服务商凭证配置
  cloudflare: {
    label: 'Cloudflare',
    fields: [
      {
        key: 'api_token',
        label: 'API Token',
        placeholder: '请输入 Cloudflare API Token',
        type: 'password',
        required: true
      }
    ]
  }
}

/**
 * 获取所有支持的服务商选项列表（用于下拉选择）
 * @returns {Array<{value: string, label: string}>}
 */
export function getProviderOptions() {
  return Object.entries(providerConfig).map(([value, config]) => ({
    value,
    label: config.label
  }))
}

/**
 * 获取指定服务商的凭证字段配置
 * @param {string} providerType - 服务商类型标识
 * @returns {Array|null} 字段配置数组，未找到返回 null
 */
export function getProviderFields(providerType) {
  return providerConfig[providerType]?.fields || null
}

/**
 * 获取服务商中文名称
 * @param {string} providerType - 服务商类型标识
 * @returns {string}
 */
export function getProviderLabel(providerType) {
  return providerConfig[providerType]?.label || providerType
}

export default providerConfig
