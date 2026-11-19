import type { StockMovementGranularity } from '../types/dashboard';
import { parseCalendarDate, type CalendarDateParts } from './dashboardDates';

// Labels of the stock movement chart. Periods and range bounds are calendar
// dates (YYYY-MM-DD); they are read as strings by parseCalendarDate and never
// turned into Date objects, so the browser's time zone cannot shift them. No
// weekday is shown. A value that is not a valid date is returned unchanged:
// these run inside Recharts formatters, where throwing would break the chart.

const pad2 = (value: number) => String(value).padStart(2, '0');

const fullDate = ({ year, month, day }: CalendarDateParts) => `${pad2(day)}/${pad2(month)}/${year}`;

/** X axis tick: DD/MM by day, MM/YYYY by month (the period is the 1st of the month). */
export function formatChartAxisLabel(period: string, granularity: StockMovementGranularity): string {
  const date = parseCalendarDate(period);
  if (!date) {
    return period;
  }
  switch (granularity) {
    case 'day':
      return `${pad2(date.day)}/${pad2(date.month)}`;
    case 'month':
      return `${pad2(date.month)}/${date.year}`;
    default:
      return period;
  }
}

/** Tooltip title: DD/MM/YYYY by day, "Tháng MM/YYYY" by month. */
export function formatChartTooltipLabel(period: string, granularity: StockMovementGranularity): string {
  const date = parseCalendarDate(period);
  if (!date) {
    return period;
  }
  switch (granularity) {
    case 'day':
      return fullDate(date);
    case 'month':
      return `Tháng ${pad2(date.month)}/${date.year}`;
    default:
      return period;
  }
}

/** The chart range, both days included: "DD/MM/YYYY – DD/MM/YYYY". */
export function formatChartRange(from: string, to: string): string {
  const format = (value: string) => {
    const date = parseCalendarDate(value);
    return date ? fullDate(date) : value;
  };
  return `${format(from)} – ${format(to)}`;
}

export const CHART_GRANULARITY_LABELS: Readonly<Record<StockMovementGranularity, string>> = {
  day: 'Theo ngày',
  month: 'Theo tháng',
};

/** "Theo ngày" or "Theo tháng"; any other value is returned unchanged. */
export function formatChartGranularity(granularity: StockMovementGranularity): string {
  return granularity === 'day' || granularity === 'month' ? CHART_GRANULARITY_LABELS[granularity] : granularity;
}
