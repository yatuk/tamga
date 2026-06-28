export interface ModelPricing {
  id: number;
  provider: string;
  model_family: string;
  model_version: string;
  input_per_1k: number;
  output_per_1k: number;
  currency: string;
  effective_from: string;
  effective_to?: string;
  source: string;
  notes?: string;
}

export interface CostBreakdownRow {
  provider: string;
  model_family: string;
  model_version: string;
  input_tokens: number;
  output_tokens: number;
  input_cost: number;
  output_cost: number;
  total_cost: number;
  currency: string;
  pricing_id: number;
}

export interface DailyCostRow {
  date: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
}

export interface CostsBreakdownResponse {
  range: string;
  daily: DailyCostRow[];
  breakdown: CostBreakdownRow[];
  total_usd: number;
  mtd_total_usd: number;
  projected_monthly_usd: number;
}
