import { useEffect, useMemo, useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, List, ListItem, ListItemText, Typography } from '@mui/material';
import { DatePicker, LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterDayjs } from '@mui/x-date-pickers/AdapterDayjs';
import dayjs from 'dayjs';
import 'dayjs/locale/es';
import fluxeandoIcon from '../../../logo/re-style/fluxeando_appicon.svg';
type Account = {
    id: string;
    name: string;
    active: boolean;
};
type Movement = {
    id: string;
    accountId: string;
    accountName: string;
    direction: string;
    categoryName: string;
    amountMinor: number;
    currency: string;
    description: string;
    occurredOn: string;
    status: string;
};
type MonthSummary = {
    month: string;
    label: string;
    income: number;
    expense: number;
};
const money = (amount: number, currency: string, hidden: boolean) => hidden ? `••••• ${currency}` : new Intl.NumberFormat('es-AR', { style: 'currency', currency: currency || 'ARS' }).format(amount / 100);
export default function AnalyticsDashboard({ movements, accounts, hidden }: {
    movements: Movement[];
    accounts: Account[];
    hidden: boolean;
}) {
    const [year, setYear] = useState(String(new Date().getFullYear()));
    const [currency, setCurrency] = useState('');
    const [accountId, setAccountId] = useState('all');
    const [selectedMonth, setSelectedMonth] = useState('');
    const [selectedDay, setSelectedDay] = useState<number | null>(null);
    const yearMovements = useMemo(() => movements.filter(m => m.status === 'active' && m.occurredOn.startsWith(year) && (accountId === 'all' || m.accountId === accountId)), [movements, year, accountId]);
    const currencies = useMemo(() => [...new Set(yearMovements.map(m => m.currency))].sort(), [yearMovements]);
    useEffect(() => { if (!currencies.includes(currency))
        setCurrency(currencies[0] ?? ''); }, [currencies, currency]);
    const scoped = useMemo(() => yearMovements.filter(m => m.currency === currency), [yearMovements, currency]);
    const selectedAccount = accountId === 'all' ? 'Consolidado (todas las cuentas)' : (accounts.find(account => account.id === accountId)?.name ?? 'Cuenta');
    const monthly = useMemo<MonthSummary[]>(() => Array.from({ length: 12 }, (_, index) => {
        const month = `${year}-${String(index + 1).padStart(2, '0')}`;
        const values = scoped.filter(m => m.occurredOn.startsWith(month));
        return { month, label: new Date(2000, index, 1).toLocaleDateString('es-AR', { month: 'short' }).replace('.', ''), income: values.filter(m => m.direction === 'income').reduce((sum, m) => sum + m.amountMinor, 0), expense: values.filter(m => m.direction === 'expense').reduce((sum, m) => sum + m.amountMinor, 0) };
    }), [scoped, year]);
    useEffect(() => { if (!monthly.some(m => m.month === selectedMonth))
        setSelectedMonth(monthly.find(m => m.income || m.expense)?.month ?? monthly[0]?.month ?? ''); }, [monthly, selectedMonth]);
    const monthMovements = scoped.filter(m => m.occurredOn.startsWith(selectedMonth));
    const daysInMonth = selectedMonth ? new Date(Number(selectedMonth.slice(0, 4)), Number(selectedMonth.slice(5, 7)), 0).getDate() : 31;
    const daily = Array.from({ length: daysInMonth }, (_, index) => {
        const day = index + 1;
        const values = monthMovements.filter(m => Number(m.occurredOn.slice(8, 10)) === day);
        return { day, values, income: values.filter(m => m.direction === 'income').reduce((sum, m) => sum + m.amountMinor, 0), expense: values.filter(m => m.direction === 'expense').reduce((sum, m) => sum + m.amountMinor, 0) };
    });
    const selectedDaily = selectedDay === null ? null : daily.find(item => item.day === selectedDay) ?? null;
    const selectedAccounts = selectedDaily ? [...new Set(selectedDaily.values.map(m => m.accountName || accounts.find(a => a.id === m.accountId)?.name || 'Cuenta sin nombre'))] : [];
    const hoverSummary = (item: typeof daily[number]) => item.values.length ? `${item.values.length} movimientos · Ingresos ${money(item.income, currency, hidden)} · Egresos ${money(item.expense, currency, hidden)}` : 'Sin movimientos';
    const yearlyDaily = useMemo(() => {
        const totals = scoped.reduce<Record<string, {
            income: number;
            expense: number;
            count: number;
        }>>((byDate, movement) => {
            const current = byDate[movement.occurredOn] ?? { income: 0, expense: 0, count: 0 };
            current[movement.direction === 'income' ? 'income' : 'expense'] += movement.amountMinor;
            current.count += 1;
            byDate[movement.occurredOn] = current;
            return byDate;
        }, {});
        const firstDay = new Date(Number(year), 0, 1);
        const leapYear = Number(year) % 4 === 0 && (Number(year) % 100 !== 0 || Number(year) % 400 === 0);
        return Array.from({ length: leapYear ? 366 : 365 }, (_, index) => {
            const date = new Date(firstDay);
            date.setDate(firstDay.getDate() + index);
            const key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
            const values = totals[key] ?? { income: 0, expense: 0, count: 0 };
            return { key, label: date.toLocaleDateString('es-AR', { day: '2-digit', month: 'short', year: 'numeric' }), ...values };
        });
    }, [scoped, year]);
    const totalIncome = monthly.reduce((sum, m) => sum + m.income, 0);
    const totalExpense = monthly.reduce((sum, m) => sum + m.expense, 0);
    const annualMax = Math.max(...monthly.flatMap(m => [m.income, m.expense]), 1);
    const dailyMax = Math.max(...daily.flatMap(item => [item.income, item.expense]), 1);
    const annualNetMax = Math.max(...monthly.map(item => Math.abs(item.income - item.expense)), 1);
    const dailyNetMax = Math.max(...daily.map(item => Math.abs(item.income - item.expense)), 1);
    const annualTrendPoints = monthly.map((item, index) => `${(index + .5) * 100 / monthly.length},${50 - ((item.income - item.expense) / annualNetMax) * 40}`).join(' ');
    const dailyTrendPoints = daily.map((item, index) => `${(index + .5) * 100 / daily.length},${50 - ((item.income - item.expense) / dailyNetMax) * 40}`).join(' ');
    const selected = monthly.find(m => m.month === selectedMonth) ?? monthly[0];
    const hasMonthlyValues = Boolean(selected?.income || selected?.expense);
    const monthlyBalancePercent = !hasMonthlyValues ? 0 : selected.income === 0 ? -100 : ((selected.income - selected.expense) / selected.income) * 100;
    const monthlyBalanceLabel = !hasMonthlyValues ? 'Sin movimientos para comparar' : Math.abs(monthlyBalancePercent) < .05 ? 'Gap 0,0% · ingresos y egresos iguales' : `Gap ${monthlyBalancePercent > 0 ? '+' : ''}${monthlyBalancePercent.toFixed(1)}% · ${monthlyBalancePercent > 0 ? 'ingresos por encima' : 'egresos por encima'}`;
    const detailTrendAngle = Math.max(-18, Math.min(18, -monthlyBalancePercent / 4));
    const previousYear = String(Number(year) - 1);
    const previousValues = movements.filter(m => m.status === 'active' && m.currency === currency && (accountId === 'all' || m.accountId === accountId) && m.occurredOn.startsWith(`${previousYear}-${selectedMonth.slice(5, 7)}`));
    const previous = { income: previousValues.filter(m => m.direction === 'income').reduce((sum, m) => sum + m.amountMinor, 0), expense: previousValues.filter(m => m.direction === 'expense').reduce((sum, m) => sum + m.amountMinor, 0) };
    const currentDate = new Date();
    const currentYear = String(currentDate.getFullYear());
    const currentMonthNumber = String(currentDate.getMonth() + 1).padStart(2, '0');
    const currentMonthName = currentDate.toLocaleDateString('es-AR', { month: 'long' });
    const historicalMonths = useMemo(() => {
        const totals = movements.filter(m => m.status === 'active' && m.currency === currency && (accountId === 'all' || m.accountId === accountId)).reduce<Record<string, {
            income: number;
            expense: number;
        }>>((byMonth, movement) => {
            const month = movement.occurredOn.slice(0, 7);
            const entry = byMonth[month] ?? { income: 0, expense: 0 };
            entry[movement.direction === 'income' ? 'income' : 'expense'] += movement.amountMinor;
            byMonth[month] = entry;
            return byMonth;
        }, {});
        return Object.entries(totals).sort(([left], [right]) => left.localeCompare(right)).map(([month, values]) => ({ month, ...values }));
    }, [movements, currency, accountId]);
    const currentMonthKey = `${currentYear}-${currentMonthNumber}`;
    const currentMonthValues = historicalMonths.find(item => item.month === currentMonthKey) ?? { income: 0, expense: 0 };
    const historicalIncomeRecord = historicalMonths.reduce<{
        month: string;
        income: number;
        expense: number;
    } | null>((record, item) => !record || item.income >= record.income ? item : record, null);
    const historicalExpenseRecord = historicalMonths.reduce<{
        month: string;
        income: number;
        expense: number;
    } | null>((record, item) => !record || item.expense >= record.expense ? item : record, null);
    const recordMonthLabel = (month?: string) => month ? new Date(`${month}-01T12:00:00`).toLocaleDateString('es-AR', { month: 'long', year: 'numeric' }) : 'Sin datos';
    const comparisonMax = Math.max(currentMonthValues.income, currentMonthValues.expense, historicalIncomeRecord?.income ?? 0, historicalExpenseRecord?.expense ?? 0, 1);
    const categoryTotals = (direction: string) => Object.entries(scoped.filter(m => m.direction === direction).reduce<Record<string, number>>((totals, m) => { const key = m.categoryName || 'Sin categoría'; totals[key] = (totals[key] ?? 0) + m.amountMinor; return totals; }, {})).sort((a, b) => b[1] - a[1]).slice(0, 5);
    const monthlyCategoryTotals = (direction: string) => Object.entries(monthMovements.filter(m => m.direction === direction).reduce<Record<string, {
        amount: number;
        count: number;
    }>>((totals, m) => { const key = m.categoryName || 'Sin categoría'; const current = totals[key] ?? { amount: 0, count: 0 }; current.amount += m.amountMinor; current.count += 1; totals[key] = current; return totals; }, {})).sort(([, left], [, right]) => right.amount - left.amount);
    const ranking = (items: [
        string,
        number
    ][], title: string) => <div className="analytics-ranking">
<h3>{title}</h3>
<p className="items">Acumulado de {year}</p>{items.length ? items.map(([name, value]) => <div className="ranking-row" key={name}>
<span>{name}</span>
<div className="ranking-track">
<i style={{ width: `${Math.max(8, value / (items[0][1] || 1) * 100)}%` }}/>
</div>
<strong>{money(value, currency, hidden)}</strong>
</div>) : <p className="items">Sin movimientos para el año seleccionado.</p>}</div>;
    return <section className="card analytics-card" data-report-period={selectedMonth}>
    <h1 className="analytics-report-title">
<img className="analytics-report-logo" src={fluxeandoIcon} alt="" aria-hidden="true"/>
<span className="analytics-report-brand"><b>FLUX</b><b>eando</b></span>
<small>Analítica financiera</small>
</h1>
    <div className="section-head">
<div>
<h2>Analítica</h2>
<p className="items">Ingresos, egresos y categorías a partir de tus movimientos.</p>
</div>
<div className="analytics-controls">
<label>Cuenta<select value={accountId} onChange={event => setAccountId(event.target.value)}>
<option value="all">Consolidado (todas)</option>{accounts.filter(a => a.active).map(a => <option key={a.id} value={a.id}>{a.name}</option>)}</select>
</label>
<LocalizationProvider dateAdapter={AdapterDayjs} adapterLocale="es">
<DatePicker views={['year']} openTo="year" value={dayjs(`${year}-01-01`)} onChange={value => { if (value?.isValid())
        setYear(value.format('YYYY')); }} slotProps={{ textField: { size: 'small', variant: 'outlined', label: 'Año' } }}/>
</LocalizationProvider>
<label>Moneda<select value={currency} onChange={event => setCurrency(event.target.value)}>{currencies.map(value => <option key={value}>{value}</option>)}</select>
</label>
</div>
</div>
    <p className="analytics-scope">Sumarización: <strong>{selectedAccount}</strong>
</p>
    {!yearMovements.length ? <p className="items">No hay movimientos activos para este año.</p> : <>
      <div className="analytics-summary">
<strong>Ingresos del año <span>{money(totalIncome, currency, hidden)}</span>
</strong>
<strong>Egresos del año <span>{money(totalExpense, currency, hidden)}</span>
</strong>
<strong>Balance <span className={totalIncome - totalExpense < 0 ? 'negative-amount' : ''}>{money(totalIncome - totalExpense, currency, hidden)}</span>
</strong>
</div>
      {historicalMonths.length > 0 && <section className="analytics-chart-block historical-month-chart">
<div className="chart-title">
<div>
<h3>Mes actual frente a TOP histórico</h3>
<span>Ingresos y egresos de {currentMonthName} comparados con sus máximos históricos · {selectedAccount}</span>
</div>
<div className="chart-legend">
<span>
<i className="legend-income"/>Ingresos</span>
<span>
<i className="legend-expense"/>Egresos</span>
</div>
</div>
<div className="record-comparison-chart">
<div className="chart-y-labels">
<span>{money(comparisonMax, currency, hidden)}</span>
<span>{money(Math.round(comparisonMax / 2), currency, hidden)}</span>
<span>$0</span>
</div>
<div className="record-comparison-bars">
<div className="chart-grid">
<i />
<i />
<i />
</div>
<div className="record-comparison-bar income">
<i style={{ height: `${currentMonthValues.income / comparisonMax * 100}%` }}/>
<strong>{money(currentMonthValues.income, currency, hidden)}</strong>
<span>Ingreso actual</span>
<small>{recordMonthLabel(currentMonthKey)}</small>
</div>
<div className="record-comparison-bar expense">
<i style={{ height: `${currentMonthValues.expense / comparisonMax * 100}%` }}/>
<strong>{money(currentMonthValues.expense, currency, hidden)}</strong>
<span>Egreso actual</span>
<small>{recordMonthLabel(currentMonthKey)}</small>
</div>
<div className="record-comparison-bar income">
<i style={{ height: `${(historicalIncomeRecord?.income ?? 0) / comparisonMax * 100}%` }}/>
<strong>{money(historicalIncomeRecord?.income ?? 0, currency, hidden)}</strong>
<span>TOP ingreso</span>
<small>{recordMonthLabel(historicalIncomeRecord?.month)}</small>
</div>
<div className="record-comparison-bar expense">
<i style={{ height: `${(historicalExpenseRecord?.expense ?? 0) / comparisonMax * 100}%` }}/>
<strong>{money(historicalExpenseRecord?.expense ?? 0, currency, hidden)}</strong>
<span>TOP egreso</span>
<small>{recordMonthLabel(historicalExpenseRecord?.month)}</small>
</div>
</div>
</div>
</section>}
      <div className="analytics-chart-block">
<div className="chart-title">
<div>
<h3>Evolución anual</h3>
<span>Comparación mensual de ingresos y egresos</span>
</div>
<div className="chart-legend">
<span>
<i className="legend-income"/>Ingresos</span>
<span>
<i className="legend-expense"/>Egresos</span>
<span>
<i className="legend-net"/>Balance neto</span>
</div>
</div>
<div className="annual-chart">
<div className="chart-y-labels">
<span>{money(annualMax, currency, hidden)}</span>
<span>{money(Math.round(annualMax / 2), currency, hidden)}</span>
<span>$0</span>
</div>
<div className="chart-plot">
<div className="chart-grid">
<i />
<i />
<i />
</div>
<div className="chart-bars">{monthly.map(m => <button className={`month-column ${m.month === selectedMonth ? 'selected' : ''}`} key={m.month} onClick={() => { setSelectedMonth(m.month); setSelectedDay(null); }} aria-label={`Seleccionar ${m.label}`}>
<span className="bar-group">
<i className="bar-income" style={{ height: `${m.income / annualMax * 100}%` }}/>
<i className="bar-expense" style={{ height: `${m.expense / annualMax * 100}%` }}/>
</span>
<small>{m.label}</small>
</button>)}</div>
<svg className="trend-line" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="Evolución mensual del balance neto">
<polyline points={annualTrendPoints}/>
</svg>
</div>
</div>
</div>
      <div className="analytics-print-page-header" aria-hidden="true">
<span><b>FLUX</b><b>eando</b></span>
<small>Analítica financiera</small>
</div>
      <div className="analytics-chart-block daily-detail">
<div className="chart-title">
<div>
<h3>Movimientos por día</h3>
<span>{selectedMonth} · {selectedAccount}</span>
</div>
<div className="chart-legend">
<span>
<i className="legend-income"/>Ingresos</span>
<span>
<i className="legend-expense"/>Egresos</span>
<span>
<i className="legend-net"/>Balance neto</span>
</div>
</div>
<div className="daily-chart">{daily.map(item => <button className="daily-column" key={item.day} title={hoverSummary(item)} onClick={() => item.values.length && setSelectedDay(item.day)} disabled={!item.values.length} aria-label={item.values.length ? `Ver detalle del día ${item.day}` : `Día ${item.day} sin movimientos`}>
<span className="daily-bars">
<i className="bar-income" style={{ height: `${item.income / dailyMax * 100}%` }}/>
<i className="bar-expense" style={{ height: `${item.expense / dailyMax * 100}%` }}/>
</span>
<small>{item.day}</small>
</button>)}</div>
<svg className="daily-trend-line" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="Evolución diaria del balance neto">
<polyline points={dailyTrendPoints}/>
</svg>
</div>
      <div className="analytics-detail">
<div className="analytics-chart-block month-detail">
<div className="chart-title">
<div>
<h3>Detalle mensual</h3>
<span>{selectedMonth} · {selectedAccount}</span>
</div>
<select value={selectedMonth} onChange={event => { setSelectedMonth(event.target.value); setSelectedDay(null); }}>{monthly.map(m => <option key={m.month}>{m.month}</option>)}</select>
</div>
<div className="detail-bars">
<div className="detail-bar income" style={{ height: `${selected.income / Math.max(selected.income, selected.expense, 1) * 100}%` }}>
<b>{money(selected.income, currency, hidden)}</b>
<span>Ingresos</span>
</div>
<div className="detail-bar expense" style={{ height: `${selected.expense / Math.max(selected.income, selected.expense, 1) * 100}%` }}>
<b>{money(selected.expense, currency, hidden)}</b>
<span>Egresos</span>
</div>
<div className="detail-trend">
<div className="detail-trend-copy">
<strong className={monthlyBalancePercent < 0 ? 'negative-gap' : 'positive-gap'}>{monthlyBalanceLabel}</strong>
<span>Tendencia del balance</span>
</div>
<i style={{ transform: `rotate(${detailTrendAngle}deg)` }}/>
</div>
</div>
<section className="analytics-month-category-breakdown">
<div>
<h3>Desglose mensual por categoría</h3>
<p>{selectedMonth} · total y cantidad de movimientos que lo componen</p>
</div>
<div className="monthly-category-columns">{(['income', 'expense'] as const).map(direction => { const items = monthlyCategoryTotals(direction); const income = direction === 'income'; return <section className={income ? 'income' : 'expense'} key={direction}>
<h4>{income ? 'Ingresos' : 'Egresos'}</h4>{items.length ? <div>{items.map(([name, values]) => <article key={name}>
<span>{name}</span>
<strong>{money(values.amount, currency, hidden)}</strong>
<small>{values.count} {values.count === 1 ? 'registro' : 'registros'}</small>
</article>)}</div> : <p className="items">Sin movimientos.</p>}</section>; })}</div>
</section>
</div>
<div className="analytics-rankings">
<div className="analytics-chart-block year-compare">
<div className="chart-title">
<div>
<h3>Mes seleccionado vs año anterior</h3>
<span>{selectedMonth} · comparación con {previousYear}</span>
</div>
</div>
<div className="comparison-bars">
<div>
<i className="bar-income" style={{ height: `${previous.income / Math.max(selected.income, previous.income, 1) * 100}%` }}/>
<span>Ingreso {previousYear}</span>
</div>
<div>
<i className="bar-income" style={{ height: `${selected.income / Math.max(selected.income, previous.income, 1) * 100}%` }}/>
<span>Ingreso {year}</span>
</div>
<div>
<i className="bar-expense" style={{ height: `${previous.expense / Math.max(selected.income, selected.expense, 1) * 100}%` }}/>
<span>Egreso {previousYear}</span>
</div>
<div>
<i className="bar-expense" style={{ height: `${selected.expense / Math.max(selected.income, selected.expense, 1) * 100}%` }}/>
<span>Egreso {year}</span>
</div>
</div>
</div>{ranking(categoryTotals('expense'), 'Top 5 categorías con más egresos')}{ranking(categoryTotals('income'), 'Top 5 categorías con más ingresos')}</div>
</div>
      <section className="analytics-yearly-daily-report">
<h3>Detalle diario del año</h3>
<p>Todos los días de {year} · {selectedAccount} · {currency}</p>
<table>
<thead>
<tr>
<th>Fecha</th>
<th>Ingresos</th>
<th>Egresos</th>
<th>Balance</th>
<th>Movimientos</th>
</tr>
</thead>
<tbody>{yearlyDaily.map(day => <tr key={day.key}>
<td>{day.label}</td>
<td>{money(day.income, currency, hidden)}</td>
<td>{money(day.expense, currency, hidden)}</td>
<td className={day.income - day.expense < 0 ? 'negative-amount' : ''}>{money(day.income - day.expense, currency, hidden)}</td>
<td>{day.count}</td>
</tr>)}</tbody>
</table>
</section>
    </>}
    <Dialog className="analytics-movement-dialog" open={Boolean(selectedDaily)} onClose={() => setSelectedDay(null)} fullWidth maxWidth="md">
<DialogTitle>
<span className="analytics-modal-eyebrow">Detalle diario</span>
<span className="analytics-modal-title">Movimientos del {selectedMonth}-{String(selectedDay ?? '').padStart(2, '0')}</span>
</DialogTitle>
<DialogContent>
<section className="analytics-modal-accounts">
<span>Cuentas incluidas en la sumarización</span>
<div>{selectedAccounts.map(account => <strong key={account}>{account}</strong>)}</div>
</section>
<div className="analytics-modal-summary">
<span className="income">
<small>Ingresos</small>
<strong>{money(selectedDaily?.income ?? 0, currency, hidden)}</strong>
</span>
<span className="expense">
<small>Egresos</small>
<strong>{money(selectedDaily?.expense ?? 0, currency, hidden)}</strong>
</span>
<span className="count">
<small>Movimientos</small>
<strong>{selectedDaily?.values.length ?? 0}</strong>
</span>
</div>
<List className="analytics-movement-list" disablePadding>{selectedDaily?.values.map(m => { const income = m.direction === 'income'; return <ListItem className={`analytics-movement-item ${income ? 'income' : 'expense'}`} key={m.id} disableGutters>
<span className="analytics-movement-kind">{income ? '↑' : '↓'}</span>
<ListItemText primary={<span className="analytics-movement-primary">
<strong>{m.description || 'Sin descripción'}</strong>
<b>{money(m.amountMinor, m.currency, hidden)}</b>
</span>} secondary={<span className="analytics-movement-meta">
<span>{m.accountName || accounts.find(a => a.id === m.accountId)?.name || 'Cuenta sin nombre'}</span>
<span>{m.categoryName || 'Sin categoría'}</span>
<span>{income ? 'Ingreso' : 'Egreso'}</span>
</span>}/>
</ListItem>; })}</List>
</DialogContent>
<DialogActions>
<Button variant="contained" onClick={() => setSelectedDay(null)}>Cerrar detalle</Button>
</DialogActions>
</Dialog>
  </section>;
}
