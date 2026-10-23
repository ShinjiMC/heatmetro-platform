// src/TimelineBarChart.js
import React, {
  useMemo,
  useRef,
  useEffect,
  useLayoutEffect,
  useState,
} from "react";
import { Bar } from "react-chartjs-2";
import PropTypes from "prop-types";

// --- Constantes de Color ---
const COLOR_BAR = "rgba(96, 165, 250, 0.8)";
const COLOR_BAR_BG = "rgba(96, 165, 250, 0.2)";
const COLOR_SELECTED = "rgba(34, 211, 238, 1)";
const COLOR_SELECTED_BG = "rgba(34, 211, 238, 0.9)";
const COLOR_ZERO = "rgba(156, 163, 175, 0.6)";
const COLOR_GRID = "rgba(255, 255, 255, 0.1)";
const COLOR_TEXT = "#fff";

// --- PLUGIN WIIFLOW ---
const dockEffectPlugin = {
  id: "dockEffect",
  beforeDatasetDraw(chart, args, options) {
    const meta = chart.getDatasetMeta(0);
    if (!meta || !meta.data) return;

    const yScale = chart.scales.y;
    const dataset = chart.data.datasets[0];
    const chartArea = chart.chartArea;

    // 1. Recalcular altura real en la escala actual y restaurar ancho
    meta.data.forEach((bar, index) => {
      const value = dataset.data[index];
      bar.y = yScale.getPixelForValue(value);

      if (bar._scaledX && bar._scaledX !== 1) {
        bar.width = bar.width / bar._scaledX;
        bar._scaledX = 1;
      }
    });

    // 2. Sin hover o sin expandido -> no aplicar lupa
    const activeElements = chart.getActiveElements();
    if (!options.enabled || activeElements.length === 0) return;

    const hoveredIndex = activeElements[0].index;

    // 3. Aplicar efecto lupa con tope en el chartArea
    meta.data.forEach((bar, index) => {
      const distance = Math.abs(index - hoveredIndex);
      const scaleY = 1 + 0.8 * Math.exp(-Math.pow(distance / 1.5, 2));
      const scaleX = 1 + 0.6 * Math.exp(-Math.pow(distance / 1.5, 2));

      if (scaleY > 1.01) {
        const height = bar.base - bar.y;
        if (height <= 0) return; // Barra en 0 no crece

        const maxHeight = bar.base - chartArea.top;
        const newHeight = Math.min(height * scaleY, maxHeight);

        bar.y = bar.base - newHeight;
        bar.width = bar.width * scaleX;
        bar._scaledX = scaleX;
      }
    });
  },
};

function TimelineBarChart({
  manifest,
  timelineData,
  selectedSha,
  onShaSelect,
  isExpanded,
}) {
  const chartRef = useRef(null);
  const chartContainerRef = useRef(null);
  const [chartSize, setChartSize] = useState({ width: 0, height: 0 });

  useLayoutEffect(() => {
    const el = chartContainerRef.current;
    if (!el) return;

    const update = () => {
      const rect = el.getBoundingClientRect();
      const w = Math.round(rect.width);
      const h = Math.round(rect.height);
      if (w > 0 && h > 0) {
        setChartSize((prev) =>
          prev.width === w && prev.height === h ? prev : { width: w, height: h }
        );
      }
    };

    update();
    let ro;
    if (typeof ResizeObserver !== "undefined") {
      ro = new ResizeObserver(update);
      ro.observe(el);
    }
    window.addEventListener("resize", update);
    const t1 = setTimeout(update, 100);
    const t2 = setTimeout(update, 600);

    return () => {
      ro?.disconnect();
      window.removeEventListener("resize", update);
      clearTimeout(t1);
      clearTimeout(t2);
    };
  }, [isExpanded]);

  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    if (chartSize.width > 0 && chartSize.height > 0) {
      try {
        chart.resize(chartSize.width, chartSize.height);
        chart.update("none");
      } catch (e) {
        console.error("Error resizing chart:", e);
      }
    }
  }, [chartSize]);

  const maxLoc = useMemo(() => {
    const locs = timelineData.map((d) => d.loc || 0);
    return Math.max(...locs, 1);
  }, [timelineData]);

  const chartData = useMemo(() => {
    const locMap = new Map(timelineData.map((d) => [d.sha_text, d.loc || 0]));
    const labels = [];
    const data = [];
    const backgroundColors = [];
    const borderColors = [];

    for (let i = manifest.length - 1; i >= 0; i--) {
      const sha = manifest[i];
      const loc = locMap.get(sha.sha_text) ?? 0;
      labels.push(sha.sha_text);
      data.push(loc);

      if (sha.sha_text === selectedSha) {
        backgroundColors.push(COLOR_SELECTED_BG);
        borderColors.push(COLOR_SELECTED);
      } else if (loc === 0) {
        backgroundColors.push(COLOR_ZERO);
        borderColors.push(COLOR_ZERO);
      } else {
        backgroundColors.push(
          isExpanded ? COLOR_BAR_BG : "rgba(96, 165, 250, 0.9)"
        );
        borderColors.push(COLOR_BAR);
      }
    }

    return {
      labels,
      datasets: [
        {
          label: "Lines of Code (LOC)",
          data,
          backgroundColor: backgroundColors,
          borderColor: borderColors,
          borderWidth: 1,
          barPercentage: 0.95,
          categoryPercentage: 1.0,
          borderRadius: { topLeft: 3, topRight: 3 },
          hoverBackgroundColor: "rgba(255, 255, 255, 0.9)",
          hoverBorderColor: "#fff",
          hoverBorderWidth: 2,
        },
      ],
    };
  }, [manifest, timelineData, selectedSha, isExpanded]);

  const selectedShaData = useMemo(
    () => manifest.find((d) => d.sha_text === selectedSha),
    [manifest, selectedSha]
  );

  // 4) Opciones
  const options = {
    responsive: false,
    maintainAspectRatio: false,
    animation: false,
    indexAxis: "x",
    interaction: {
      mode: "index",
      intersect: false,
    },
    plugins: {
      legend: { display: false },
      dockEffect: {
        enabled: isExpanded,
      },
      tooltip: {
        enabled: true,
        backgroundColor: "rgba(17, 24, 39, 0.95)",
        titleFont: { size: 13, family: "monospace" },
        bodyFont: { size: 12 },
        padding: 10,
        cornerRadius: 6,
        displayColors: false,
        callbacks: {
          title: (items) =>
            `Commit: ${chartData.labels[items[0].dataIndex].substring(0, 7)}`,
          label: (ctx) => `Lines of Code: ${ctx.raw.toLocaleString()}`,
        },
      },
    },
    scales: {
      y: {
        type: "linear",
        min: 0,
        max: maxLoc * 1.1,
        ticks: {
          color: isExpanded ? COLOR_TEXT : "transparent",
          font: { size: 10 },
          maxTicksLimit: 4, // <-- Limita la cantidad de ticks para ahorrar espacio
          callback: (value) => {
            // <-- CORRECCIÓN 2: Formateo limpio sin decimales infinitos
            if (value === 0) return "0";
            if (value >= 1000000)
              return `${Number((value / 1000000).toFixed(1))}M`;
            if (value >= 1000) return `${Number((value / 1000).toFixed(1))}k`;
            return value.toString();
          },
        },
        grid: { color: isExpanded ? COLOR_GRID : "transparent" },
      },
      x: {
        ticks: { display: false },
        grid: {
          display: false,
          drawBorder: true,
          borderColor: isExpanded
            ? "rgba(255, 255, 255, 0.1)"
            : "rgba(34, 211, 238, 0.8)",
          borderWidth: isExpanded ? 1 : 2,
        },
      },
    },
    onClick: (evt, elements) => {
      if (elements.length > 0) {
        const index = elements[0].index;
        const sha = chartData.labels[index];
        const loc = chartData.datasets[0].data[index];
        if (loc > 0 && sha !== selectedSha) onShaSelect(sha);
      }
    },
    onHover: (event, chartElement) => {
      if (event.native) {
        event.native.target.style.cursor = chartElement[0]
          ? "pointer"
          : "default";
      }
    },
  };

  return (
    <div
      className={`w-full h-full flex flex-col justify-between ${
        isExpanded
          ? "p-2 bg-transparent text-white rounded-xl"
          : "p-1 bg-transparent"
      }`}
    >
      <div
        ref={chartContainerRef}
        className="relative w-full min-w-0"
        style={{ height: isExpanded ? "80%" : "82%", overflow: "visible" }}
      >
        {chartSize.width > 0 && chartSize.height > 0 && (
          <Bar
            ref={chartRef}
            data={chartData}
            options={options}
            width={chartSize.width}
            height={chartSize.height}
            plugins={[dockEffectPlugin]}
          />
        )}
      </div>

      {selectedShaData && (
        <div
          className={`w-full text-center ${
            isExpanded ? "text-gray-400 " : "text-white text-shadow-lg"
          }`}
          style={{ height: isExpanded ? "20%" : "18%" }}
        >
          <p
            className={`text-xs font-mono truncate ${
              isExpanded
                ? "text-gray-300"
                : "text-white font-semibold text-[10px]"
            }`}
          >
            {selectedShaData.author_name}
          </p>
          <p className="text-[9px] opacity-80 truncate leading-none">
            {new Date(selectedShaData.commit_date).toLocaleDateString()}
          </p>
        </div>
      )}
    </div>
  );
}

TimelineBarChart.propTypes = {
  manifest: PropTypes.array.isRequired,
  timelineData: PropTypes.array.isRequired,
  selectedSha: PropTypes.string,
  onShaSelect: PropTypes.func.isRequired,
  isExpanded: PropTypes.bool.isRequired,
};

export default TimelineBarChart;
