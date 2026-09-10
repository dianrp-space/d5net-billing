// Tree-shaken ECharts setup: register only the charts/components the app uses,
// so the charts bundle stays small instead of pulling the full echarts build.
import * as echarts from "echarts/core";
import { BarChart, PieChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

echarts.use([BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer]);

export default echarts;
