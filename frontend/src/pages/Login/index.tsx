import { Button, Card, Form, Input, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'

import { useAuthStore } from '@/stores/auth'

interface LoginValues {
  username: string
  password: string
}

/**
 * 登录页(T2 接入真实接口前的骨架):
 * 提交后写入演示会话以走通"登录 → 布局"流程,真实 POST /api/v1/auth/login 随 T2 落地。
 */
export default function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)

  const onFinish = (values: LoginValues) => {
    setSession(`dev-${values.username}`, values.username)
    navigate('/', { replace: true })
  }

  return (
    <div className="login-page">
      <Card title={t('login.title')} className="login-card">
        <Form<LoginValues> onFinish={onFinish} layout="vertical" requiredMark={false}>
          <Form.Item
            name="username"
            label={t('login.username')}
            rules={[{ required: true, message: t('login.usernameRequired') }]}
          >
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label={t('login.password')}
            rules={[{ required: true, message: t('login.passwordRequired') }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block>
            {t('login.submit')}
          </Button>
          <Typography.Text
            type="secondary"
            style={{ display: 'block', marginTop: 12, fontSize: 12 }}
          >
            {t('login.devHint')}
          </Typography.Text>
        </Form>
      </Card>
    </div>
  )
}
