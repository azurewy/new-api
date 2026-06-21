/*
Copyright (C) 2023-2026 QuantumNous

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
import { z } from 'zod'
import type { TFunction } from 'i18next'
import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'
import type { SubscriptionPlan, PlanPayload } from '../types'

const MANAGED_QUOTA_LIMITS = [
  {
    key: '5h',
    name: '5 hour quota',
    amountField: 'quota_policy_5h_amount',
    window_seconds: 18_000,
    reset: 'rolling',
  },
  {
    key: '7d',
    name: '7 day quota',
    amountField: 'quota_policy_7d_amount',
    window_seconds: 604_800,
    reset: 'rolling',
  },
  {
    key: 'monthly',
    name: 'Monthly quota',
    amountField: 'quota_policy_monthly_amount',
    window_seconds: 2_592_000,
    reset: 'subscription_cycle',
  },
] as const

type ManagedQuotaAmountField =
  (typeof MANAGED_QUOTA_LIMITS)[number]['amountField']

type QuotaPolicyLimit = {
  key?: unknown
  name?: unknown
  amount?: unknown
  window_seconds?: unknown
  reset?: unknown
  [key: string]: unknown
}

type QuotaPolicy = {
  mode?: unknown
  unit?: unknown
  limits?: unknown
  [key: string]: unknown
}

function parseQuotaPolicy(policyJSON?: string): QuotaPolicy | undefined {
  const raw = policyJSON?.trim()
  if (!raw) return undefined
  try {
    const parsed: unknown = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as QuotaPolicy
    }
  } catch {
    return undefined
  }
  return undefined
}

function coerceQuotaAmount(value: unknown): number {
  const amount = Number(value || 0)
  if (!Number.isFinite(amount) || amount <= 0) return 0
  return Math.round(amount)
}

function getPositiveQuotaAmount(value: unknown): number | undefined {
  const amount = coerceQuotaAmount(value)
  return amount > 0 ? amount : undefined
}

function getManagedQuotaAmounts(
  policyJSON?: string
): Record<ManagedQuotaAmountField, number> {
  const policy = parseQuotaPolicy(policyJSON)
  const limits = Array.isArray(policy?.limits)
    ? (policy.limits as QuotaPolicyLimit[])
    : []
  const values = Object.fromEntries(
    MANAGED_QUOTA_LIMITS.map((limit) => [limit.amountField, 0])
  ) as Record<ManagedQuotaAmountField, number>

  for (const limit of MANAGED_QUOTA_LIMITS) {
    const match = limits.find((item) => item?.key === limit.key)
    values[limit.amountField] = coerceQuotaAmount(match?.amount)
  }

  return values
}

function buildManagedQuotaPolicyJSON(
  policyJSON: string | undefined,
  amounts: Record<ManagedQuotaAmountField, number>
): string {
  const rawPolicy = policyJSON?.trim() || ''
  const existingPolicy = parseQuotaPolicy(policyJSON)
  const existingLimits = Array.isArray(existingPolicy?.limits)
    ? (existingPolicy.limits as QuotaPolicyLimit[])
    : []
  const managedKeys = new Set<string>(
    MANAGED_QUOTA_LIMITS.map((limit) => limit.key)
  )
  const preservedLimits = existingLimits.filter(
    (limit) => typeof limit?.key !== 'string' || !managedKeys.has(limit.key)
  )
  const managedLimits = MANAGED_QUOTA_LIMITS.map((limit) => ({
    key: limit.key,
    name: limit.name,
    amount: coerceQuotaAmount(amounts[limit.amountField]),
    window_seconds: limit.window_seconds,
    reset: limit.reset,
  })).filter((limit) => limit.amount > 0)

  const limits = [...preservedLimits, ...managedLimits]
  if (limits.length === 0) return rawPolicy

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
  })
}

export function getPlanFormSchema(t: TFunction) {
  return z.object({
    title: z.string().min(1, t('Please enter plan title')),
    subtitle: z.string().optional(),
    price_amount: z.coerce.number().min(0, t('Please enter amount')),
    duration_unit: z.enum(['year', 'month', 'day', 'hour', 'custom']),
    duration_value: z.coerce.number().min(1),
    custom_seconds: z.coerce.number().min(0).optional(),
    quota_reset_period: z.enum([
      'never',
      'daily',
      'weekly',
      'monthly',
      'custom',
    ]),
    quota_reset_custom_seconds: z.coerce.number().min(0).optional(),
    enabled: z.boolean(),
    sort_order: z.coerce.number(),
    allow_balance_pay: z.boolean(),
    max_purchase_per_user: z.coerce.number().min(0),
    total_amount: z.coerce.number().min(0),
    quota_policy: z.string().optional(),
    quota_policy_5h_amount: z.coerce.number().min(0),
    quota_policy_7d_amount: z.coerce.number().min(0),
    quota_policy_monthly_amount: z.coerce.number().min(0),
    upgrade_group: z.string().optional(),
    stripe_price_id: z.string().optional(),
    creem_product_id: z.string().optional(),
    waffo_pancake_product_id: z.string().optional(),
  })
}

export type PlanFormValues = z.infer<ReturnType<typeof getPlanFormSchema>>

export const PLAN_FORM_DEFAULTS: PlanFormValues = {
  title: '',
  subtitle: '',
  price_amount: 0,
  duration_unit: 'month',
  duration_value: 1,
  custom_seconds: 0,
  quota_reset_period: 'never',
  quota_reset_custom_seconds: 0,
  enabled: true,
  sort_order: 0,
  allow_balance_pay: true,
  max_purchase_per_user: 0,
  total_amount: 0,
  quota_policy: '',
  quota_policy_5h_amount: 0,
  quota_policy_7d_amount: 0,
  quota_policy_monthly_amount: 0,
  upgrade_group: '',
  stripe_price_id: '',
  creem_product_id: '',
  waffo_pancake_product_id: '',
}

export function planToFormValues(plan: SubscriptionPlan): PlanFormValues {
  const explicit5hAmount = getPositiveQuotaAmount(plan.quota_policy_5h_amount)
  const explicit7dAmount = getPositiveQuotaAmount(plan.quota_policy_7d_amount)
  const explicitMonthlyAmount = getPositiveQuotaAmount(
    plan.quota_policy_monthly_amount
  )
  const quotaPolicyAmounts = {
    ...getManagedQuotaAmounts(plan.quota_policy),
    ...(explicit5hAmount !== undefined
      ? { quota_policy_5h_amount: explicit5hAmount }
      : {}),
    ...(explicit7dAmount !== undefined
      ? { quota_policy_7d_amount: explicit7dAmount }
      : {}),
    ...(explicitMonthlyAmount !== undefined
      ? { quota_policy_monthly_amount: explicitMonthlyAmount }
      : {}),
  }

  return {
    title: plan.title || '',
    subtitle: plan.subtitle || '',
    price_amount: Number(plan.price_amount || 0),
    duration_unit: plan.duration_unit || 'month',
    duration_value: Number(plan.duration_value || 1),
    custom_seconds: Number(plan.custom_seconds || 0),
    quota_reset_period: plan.quota_reset_period || 'never',
    quota_reset_custom_seconds: Number(plan.quota_reset_custom_seconds || 0),
    enabled: plan.enabled !== false,
    sort_order: Number(plan.sort_order || 0),
    allow_balance_pay: plan.allow_balance_pay !== false,
    max_purchase_per_user: Number(plan.max_purchase_per_user || 0),
    total_amount: quotaUnitsToDollars(Number(plan.total_amount || 0)),
    quota_policy: plan.quota_policy || '',
    ...quotaPolicyAmounts,
    upgrade_group: plan.upgrade_group || '',
    stripe_price_id: plan.stripe_price_id || '',
    creem_product_id: plan.creem_product_id || '',
    waffo_pancake_product_id: plan.waffo_pancake_product_id || '',
  }
}

export function formValuesToPlanPayload(values: PlanFormValues): PlanPayload {
  const {
    quota_policy_5h_amount,
    quota_policy_7d_amount,
    quota_policy_monthly_amount,
    ...planValues
  } = values

  return {
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
      sort_order: Number(values.sort_order || 0),
      max_purchase_per_user: Number(values.max_purchase_per_user || 0),
      total_amount: parseQuotaFromDollars(Number(values.total_amount || 0)),
      quota_policy: buildManagedQuotaPolicyJSON(values.quota_policy, {
        quota_policy_5h_amount,
        quota_policy_7d_amount,
        quota_policy_monthly_amount,
      }),
      upgrade_group: values.upgrade_group || '',
    },
  }
}
