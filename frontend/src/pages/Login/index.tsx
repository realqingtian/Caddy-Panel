import { Button, Card, Form, Input, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useState } from 'react'

import { login, apiErrorMessage } from '@/api/auth'
import { App } from 'antd'
import { useAuthStore } from '@/stores/auth'

interface LoginValues {
  username: string
  password: string
}

/** 登录页:调用 POST /api/v1/auth/login;错误消息为后端 i18n 文案(FR-101/103)。 */
export default function LoginPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const [submitting, setSubmitting] = useState(false)

  const onFinish = async (values: LoginValues) => {
    setSubmitting(true)
    try {
      const result = await login(values.username, values.password)
      setSession(result.token, result.username)
      navigate('/', { replace: true })
    } catch (error) {
      message.error(apiErrorMessage(error, t('login.failed')))
    } finally {
      setSubmitting(false)
    }
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
          <Button type="primary" htmlType="submit" block loading={submitting}>
            {t('login.submit')}
          </Button>
          <Typography.Text
            type="secondary"
            style={{ display: 'block', marginTop: 12, fontSize: 12 }}
          >
            {t('login.firstRunHint')}
          </Typography.Text>
        </Form>
      </Card>
    </div>
  )
}
