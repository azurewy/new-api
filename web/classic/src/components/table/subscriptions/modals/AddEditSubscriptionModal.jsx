/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState, useRef } from 'react';
import {
  Avatar,
  Button,
  Card,
  Col,
  Form,
  Row,
  Select,
  SideSheet,
  Space,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconCalendarClock,
  IconClose,
  IconCreditCard,
  IconSave,
} from '@douyinfe/semi-icons';
import { Clock, RefreshCw } from 'lucide-react';
import { API, showError, showSuccess } from '../../../../helpers';
import {
  quotaToDisplayAmount,
  displayAmountToQuota,
} from '../../../../helpers/quota';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';

const { Text, Title } = Typography;

const durationUnitOptions = [
  { value: 'year', label: '年' },
  { value: 'month', label: '月' },
  { value: 'day', label: '日' },
  { value: 'hour', label: '小时' },
  { value: 'custom', label: '自定义(秒)' },
];

const resetPeriodOptions = [
  { value: 'never', label: '不重置' },
  { value: 'daily', label: '每天' },
  { value: 'weekly', label: '每周' },
  { value: 'monthly', label: '每月' },
  { value: 'custom', label: '自定义(秒)' },
];

const managedQuotaLimits = [
  {
    key: '5h',
    name: '5 小时额度',
    amountField: 'quota_policy_5h_amount',
    window_seconds: 18000,
    reset: 'rolling',
  },
  {
    key: '7d',
    name: '7 天额度',
    amountField: 'quota_policy_7d_amount',
    window_seconds: 604800,
    reset: 'rolling',
  },
  {
    key: 'monthly',
    name: '月总额度',
    amountField: 'quota_policy_monthly_amount',
    window_seconds: 2592000,
    reset: 'subscription_cycle',
  },
];

const parseQuotaPolicy = (policyJSON) => {
  const raw = policyJSON?.trim();
  if (!raw) return undefined;
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed;
    }
  } catch {
    return undefined;
  }
  return undefined;
};

const coerceQuotaAmount = (value) => {
  const amount = Number(value || 0);
  if (!Number.isFinite(amount) || amount <= 0) return 0;
  return Math.round(amount);
};

const getManagedQuotaAmounts = (policyJSON) => {
  const policy = parseQuotaPolicy(policyJSON);
  const limits = Array.isArray(policy?.limits) ? policy.limits : [];
  return managedQuotaLimits.reduce((acc, limit) => {
    const match = limits.find((item) => item?.key === limit.key);
    acc[limit.amountField] = coerceQuotaAmount(match?.amount);
    return acc;
  }, {});
};

const getPositiveQuotaAmount = (value) => {
  const amount = coerceQuotaAmount(value);
  return amount > 0 ? amount : undefined;
};

const buildManagedQuotaPolicyJSON = (policyJSON, amounts) => {
  const rawPolicy = policyJSON?.trim() || '';
  const existingPolicy = parseQuotaPolicy(policyJSON);
  const existingLimits = Array.isArray(existingPolicy?.limits)
    ? existingPolicy.limits
    : [];
  const managedKeys = new Set(managedQuotaLimits.map((limit) => limit.key));
  const preservedLimits = existingLimits.filter(
    (limit) => typeof limit?.key !== 'string' || !managedKeys.has(limit.key),
  );
  const managedLimits = managedQuotaLimits
    .map((limit) => ({
      key: limit.key,
      name: limit.name,
      amount: coerceQuotaAmount(amounts[limit.amountField]),
      window_seconds: limit.window_seconds,
      reset: limit.reset,
    }))
    .filter((limit) => limit.amount > 0);

  const limits = [...preservedLimits, ...managedLimits];
  if (limits.length === 0) return rawPolicy;

  return JSON.stringify({
    ...(existingPolicy || {}),
    mode:
      typeof existingPolicy?.mode === 'string' && existingPolicy.mode.trim()
        ? existingPolicy.mode
        : 'all_limits_required',
    unit:
      typeof existingPolicy?.unit === 'string' && existingPolicy.unit.trim()
        ? existingPolicy.unit
        : 'quota',
    limits,
  });
};

const AddEditSubscriptionModal = ({
  visible,
  handleClose,
  editingPlan,
  placement = 'left',
  refresh,
  t,
}) => {
  const [loading, setLoading] = useState(false);
  const [groupOptions, setGroupOptions] = useState([]);
  const [groupLoading, setGroupLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const isEdit = editingPlan?.plan?.id !== undefined;
  const currentPlan = editingPlan?.plan || {};
  const formKey = isEdit
    ? [
        'edit',
        currentPlan.id,
        currentPlan.updated_at || 0,
        currentPlan.quota_policy || '',
        currentPlan.quota_policy_5h_amount || 0,
        currentPlan.quota_policy_7d_amount || 0,
        currentPlan.quota_policy_monthly_amount || 0,
      ].join('-')
    : 'create';

  const getInitValues = () => ({
    title: '',
    subtitle: '',
    price_amount: 0,
    currency: 'USD',
    duration_unit: 'month',
    duration_value: 1,
    custom_seconds: 0,
    quota_reset_period: 'never',
    quota_reset_custom_seconds: 0,
    quota_policy: '',
    quota_policy_5h_amount: 0,
    quota_policy_7d_amount: 0,
    quota_policy_monthly_amount: 0,
    enabled: true,
    sort_order: 0,
    max_purchase_per_user: 0,
    total_amount: 0,
    upgrade_group: '',
    stripe_price_id: '',
    creem_product_id: '',
  });

  const buildFormValues = () => {
    const base = getInitValues();
    if (editingPlan?.plan?.id === undefined) return base;
    const p = editingPlan.plan || {};
    const explicit5hAmount = getPositiveQuotaAmount(p.quota_policy_5h_amount);
    const explicit7dAmount = getPositiveQuotaAmount(p.quota_policy_7d_amount);
    const explicitMonthlyAmount = getPositiveQuotaAmount(
      p.quota_policy_monthly_amount,
    );
    const quotaPolicyAmounts = {
      ...getManagedQuotaAmounts(p.quota_policy),
      ...(explicit5hAmount !== undefined
        ? { quota_policy_5h_amount: explicit5hAmount }
        : {}),
      ...(explicit7dAmount !== undefined
        ? { quota_policy_7d_amount: explicit7dAmount }
        : {}),
      ...(explicitMonthlyAmount !== undefined
        ? { quota_policy_monthly_amount: explicitMonthlyAmount }
        : {}),
    };
    return {
      ...base,
      title: p.title || '',
      subtitle: p.subtitle || '',
      price_amount: Number(p.price_amount || 0),
      currency: 'USD',
      duration_unit: p.duration_unit || 'month',
      duration_value: Number(p.duration_value || 1),
      custom_seconds: Number(p.custom_seconds || 0),
      quota_reset_period: p.quota_reset_period || 'never',
      quota_reset_custom_seconds: Number(p.quota_reset_custom_seconds || 0),
      quota_policy: p.quota_policy || '',
      ...quotaPolicyAmounts,
      enabled: p.enabled !== false,
      sort_order: Number(p.sort_order || 0),
      max_purchase_per_user: Number(p.max_purchase_per_user || 0),
      total_amount: Number(
        quotaToDisplayAmount(p.total_amount || 0).toFixed(2),
      ),
      upgrade_group: p.upgrade_group || '',
      stripe_price_id: p.stripe_price_id || '',
      creem_product_id: p.creem_product_id || '',
    };
  };

  useEffect(() => {
    if (!visible) return;
    setGroupLoading(true);
    API.get('/api/group')
      .then((res) => {
        if (res.data?.success) {
          setGroupOptions(res.data?.data || []);
        } else {
          setGroupOptions([]);
        }
      })
      .catch(() => setGroupOptions([]))
      .finally(() => setGroupLoading(false));
  }, [visible]);

  const submit = async (values) => {
    if (!values.title || values.title.trim() === '') {
      showError(t('套餐标题不能为空'));
      return;
    }
    setLoading(true);
    try {
      const {
        quota_policy_5h_amount,
        quota_policy_7d_amount,
        quota_policy_monthly_amount,
        ...planValues
      } = values;
      const payload = {
        plan: {
          ...planValues,
          price_amount: Number(values.price_amount || 0),
          currency: 'USD',
          duration_value: Number(values.duration_value || 0),
          custom_seconds: Number(values.custom_seconds || 0),
          quota_reset_period: values.quota_reset_period || 'never',
          quota_reset_custom_seconds:
            values.quota_reset_period === 'custom'
              ? Number(values.quota_reset_custom_seconds || 0)
              : 0,
          quota_policy: buildManagedQuotaPolicyJSON(values.quota_policy, {
            quota_policy_5h_amount,
            quota_policy_7d_amount,
            quota_policy_monthly_amount,
          }),
          sort_order: Number(values.sort_order || 0),
          max_purchase_per_user: Number(values.max_purchase_per_user || 0),
          total_amount: displayAmountToQuota(values.total_amount),
          upgrade_group: values.upgrade_group || '',
        },
      };
      if (editingPlan?.plan?.id) {
        const res = await API.put(
          `/api/subscription/admin/plans/${editingPlan.plan.id}`,
          payload,
        );
        if (res.data?.success) {
          showSuccess(t('更新成功'));
          handleClose();
          refresh?.();
        } else {
          showError(res.data?.message || t('更新失败'));
        }
      } else {
        const res = await API.post('/api/subscription/admin/plans', payload);
        if (res.data?.success) {
          showSuccess(t('创建成功'));
          handleClose();
          refresh?.();
        } else {
          showError(res.data?.message || t('创建失败'));
        }
      }
    } catch (e) {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <SideSheet
        placement={placement}
        title={
          <Space>
            {isEdit ? (
              <Tag color='blue' shape='circle'>
                {t('更新')}
              </Tag>
            ) : (
              <Tag color='green' shape='circle'>
                {t('新建')}
              </Tag>
            )}
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新套餐信息') : t('创建新的订阅套餐')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={visible}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={handleClose}
                icon={<IconClose />}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={handleClose}
      >
        <Spin spinning={loading}>
          <Form
            key={formKey}
            initValues={buildFormValues()}
            getFormApi={(api) => (formApiRef.current = api)}
            onSubmit={submit}
          >
            {({ values }) => (
              <div className='p-2'>
                {/* 基本信息 */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-4'>
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='blue'
                      className='mr-2 shadow-md'
                    >
                      <IconCalendarClock size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('基本信息')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('套餐的基本信息和定价')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='title'
                        label={t('套餐标题')}
                        placeholder={t('例如：基础套餐')}
                        required
                        rules={[
                          { required: true, message: t('请输入套餐标题') },
                        ]}
                        showClear
                      />
                    </Col>

                    <Col span={24}>
                      <Form.Input
                        field='subtitle'
                        label={t('套餐副标题')}
                        placeholder={t('例如：适合轻度使用')}
                        showClear
                      />
                    </Col>

                    <Col span={12}>
                      <Form.InputNumber
                        field='price_amount'
                        label={t('实付金额')}
                        required
                        min={0}
                        precision={2}
                        rules={[{ required: true, message: t('请输入金额') }]}
                        style={{ width: '100%' }}
                      />
                    </Col>

                    <Col span={12}>
                      <Form.InputNumber
                        field='total_amount'
                        label={t('总额度')}
                        required
                        min={0}
                        precision={2}
                        rules={[{ required: true, message: t('请输入总额度') }]}
                        extraText={`${t('0 表示不限')} · ${t('原生额度')}：${displayAmountToQuota(
                          values.total_amount,
                        )}`}
                        style={{ width: '100%' }}
                      />
                    </Col>

                    <Col span={12}>
                      <Form.Select
                        field='upgrade_group'
                        label={t('升级分组')}
                        showClear
                        loading={groupLoading}
                        placeholder={t('不升级')}
                        extraText={t(
                          '购买或手动新增订阅会升级到该分组；当套餐失效/过期或手动作废/删除后，将回退到升级前分组。回退不会立即生效，通常会有几分钟延迟。',
                        )}
                      >
                        <Select.Option value=''>{t('不升级')}</Select.Option>
                        {(groupOptions || []).map((g) => (
                          <Select.Option key={g} value={g}>
                            {g}
                          </Select.Option>
                        ))}
                      </Form.Select>
                    </Col>

                    <Col span={12}>
                      <Form.Input
                        field='currency'
                        label={t('币种')}
                        disabled
                        extraText={t('由全站货币展示设置统一控制')}
                      />
                    </Col>

                    <Col span={12}>
                      <Form.InputNumber
                        field='sort_order'
                        label={t('排序')}
                        precision={0}
                        style={{ width: '100%' }}
                      />
                    </Col>

                    <Col span={12}>
                      <Form.InputNumber
                        field='max_purchase_per_user'
                        label={t('购买上限')}
                        min={0}
                        precision={0}
                        extraText={t('0 表示不限')}
                        style={{ width: '100%' }}
                      />
                    </Col>

                    <Col span={12}>
                      <Form.Switch
                        field='enabled'
                        label={t('启用状态')}
                        size='large'
                      />
                    </Col>
                  </Row>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.TextArea
                        field='quota_policy'
                        label={t('额度策略 JSON')}
                        autosize={{ minRows: 6, maxRows: 12 }}
                        placeholder='{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":1000000,"window_seconds":18000,"reset":"rolling"}]}'
                        extraText={t(
                          '可选。官网同步的多层套餐额度会写入这里；编辑已同步套餐时请保留这段 JSON。',
                        )}
                      />
                    </Col>
                  </Row>

                  <div className='mt-4 mb-2 flex items-center justify-between'>
                    <Text strong>{t('套餐额度控制')}</Text>
                    <Text type='tertiary' size='small'>
                      {t('任一层用完即不可调用')}
                    </Text>
                  </div>
                  <Row gutter={12}>
                    <Col span={8}>
                      <Form.InputNumber
                        field='quota_policy_5h_amount'
                        label={t('5 小时额度')}
                        min={0}
                        precision={0}
                        style={{ width: '100%' }}
                      />
                    </Col>
                    <Col span={8}>
                      <Form.InputNumber
                        field='quota_policy_7d_amount'
                        label={t('7 天额度')}
                        min={0}
                        precision={0}
                        style={{ width: '100%' }}
                      />
                    </Col>
                    <Col span={8}>
                      <Form.InputNumber
                        field='quota_policy_monthly_amount'
                        label={t('月总额度')}
                        min={0}
                        precision={0}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  </Row>
                </Card>

                {/* 有效期设置 */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-4'>
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='green'
                      className='mr-2 shadow-md'
                    >
                      <Clock size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('有效期设置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('配置套餐的有效时长')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={12}>
                      <Form.Select
                        field='duration_unit'
                        label={t('有效期单位')}
                        required
                        rules={[{ required: true }]}
                      >
                        {durationUnitOptions.map((o) => (
                          <Select.Option key={o.value} value={o.value}>
                            {o.label}
                          </Select.Option>
                        ))}
                      </Form.Select>
                    </Col>

                    <Col span={12}>
                      {values.duration_unit === 'custom' ? (
                        <Form.InputNumber
                          field='custom_seconds'
                          label={t('自定义秒数')}
                          required
                          min={1}
                          precision={0}
                          rules={[{ required: true, message: t('请输入秒数') }]}
                          style={{ width: '100%' }}
                        />
                      ) : (
                        <Form.InputNumber
                          field='duration_value'
                          label={t('有效期数值')}
                          required
                          min={1}
                          precision={0}
                          rules={[{ required: true, message: t('请输入数值') }]}
                          style={{ width: '100%' }}
                        />
                      )}
                    </Col>
                  </Row>
                </Card>

                {/* 额度重置 */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-4'>
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='orange'
                      className='mr-2 shadow-md'
                    >
                      <RefreshCw size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('额度重置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('支持周期性重置套餐权益额度')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={12}>
                      <Form.Select
                        field='quota_reset_period'
                        label={t('重置周期')}
                      >
                        {resetPeriodOptions.map((o) => (
                          <Select.Option key={o.value} value={o.value}>
                            {o.label}
                          </Select.Option>
                        ))}
                      </Form.Select>
                    </Col>
                    <Col span={12}>
                      {values.quota_reset_period === 'custom' ? (
                        <Form.InputNumber
                          field='quota_reset_custom_seconds'
                          label={t('自定义秒数')}
                          required
                          min={60}
                          precision={0}
                          rules={[{ required: true, message: t('请输入秒数') }]}
                          style={{ width: '100%' }}
                        />
                      ) : (
                        <Form.InputNumber
                          field='quota_reset_custom_seconds'
                          label={t('自定义秒数')}
                          min={0}
                          precision={0}
                          style={{ width: '100%' }}
                          disabled
                        />
                      )}
                    </Col>
                  </Row>
                </Card>

                {/* 第三方支付配置 */}
                <Card className='!rounded-2xl shadow-sm border-0 mb-4'>
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='purple'
                      className='mr-2 shadow-md'
                    >
                      <IconCreditCard size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('第三方支付配置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('Stripe/Creem 商品ID（可选）')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='stripe_price_id'
                        label='Stripe PriceId'
                        placeholder='price_...'
                        showClear
                      />
                    </Col>

                    <Col span={24}>
                      <Form.Input
                        field='creem_product_id'
                        label='Creem ProductId'
                        placeholder='prod_...'
                        showClear
                      />
                    </Col>
                  </Row>
                </Card>
              </div>
            )}
          </Form>
        </Spin>
      </SideSheet>
    </>
  );
};

export default AddEditSubscriptionModal;
