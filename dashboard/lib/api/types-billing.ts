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
  /** The model name as the client sent it. */
  model?: string;
  /** False when the model has no price: the cost fields are 0 and mean "unknown". */
  priced?: boolean;
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
  /** False when the model has no price: cost_usd is 0 and means "unknown". */
  priced?: boolean;
}

/** A model with usage but no price. Its tokens are in no USD total. */
export interface UnpricedModel {
  provider: string;
  model: string;
  tokens: number;
}

export interface CostsBreakdownResponse {
  range: string;
  daily: DailyCostRow[];
  breakdown: CostBreakdownRow[];
  unpriced?: UnpricedModel[];
  total_usd: number;
  mtd_total_usd: number;
  projected_monthly_usd: number;
}
