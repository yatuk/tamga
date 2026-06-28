"""PDF report generator � OWASP + incident reports via ReportLab."""

from __future__ import annotations

import io
from datetime import datetime, timezone
from typing import Any

import structlog

from app.compliance import (
    OWASP_LLM_TOP_10_2025,
    PRIVACY_ENTITIES,
    compute_compliance_report,
)

logger = structlog.get_logger()

_REPORTLAB_AVAILABLE = False
try:
    from reportlab.lib import colors as rl_colors
    from reportlab.lib.pagesizes import A4
    from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
    from reportlab.lib.units import cm, mm
    from reportlab.platypus import (
        BaseDocTemplate,
        Frame,
        PageTemplate,
        Paragraph,
        SimpleDocTemplate,
        Spacer,
        Table,
        TableStyle,
    )

    _REPORTLAB_AVAILABLE = True
except ImportError:
    logger.warning("ReportLab not installed — PDF reports unavailable, JSON-only mode")


# Tamga brand colors
_TAMGA_NAVY = rl_colors.HexColor("#0F172A") if _REPORTLAB_AVAILABLE else None
_TAMGA_BLUE = rl_colors.HexColor("#0369A1") if _REPORTLAB_AVAILABLE else None
_TAMGA_BG = rl_colors.HexColor("#F8FAFC") if _REPORTLAB_AVAILABLE else None
_TAMGA_RED = rl_colors.HexColor("#DC2626") if _REPORTLAB_AVAILABLE else None
_TAMGA_GREEN = rl_colors.HexColor("#16A34A") if _REPORTLAB_AVAILABLE else None
_TAMGA_AMBER = rl_colors.HexColor("#D97706") if _REPORTLAB_AVAILABLE else None


def _build_pdf_styles() -> dict[str, ParagraphStyle]:
    """Build Tamga-branded paragraph styles."""
    styles = getSampleStyleSheet()
    styles.add(ParagraphStyle(
        "TamgaTitle", parent=styles["Title"],
        fontName="Helvetica-Bold", fontSize=22, textColor=_TAMGA_NAVY,
        spaceAfter=6 * mm,
    ))
    styles.add(ParagraphStyle(
        "TamgaH2", parent=styles["Heading2"],
        fontName="Helvetica-Bold", fontSize=14, textColor=_TAMGA_BLUE,
        spaceBefore=8 * mm, spaceAfter=3 * mm,
    ))
    styles.add(ParagraphStyle(
        "TamgaBody", parent=styles["Normal"],
        fontName="Helvetica", fontSize=9, leading=13,
        textColor=rl_colors.HexColor("#334155"),
    ))
    styles.add(ParagraphStyle(
        "TamgaSmall", parent=styles["Normal"],
        fontName="Helvetica", fontSize=7, leading=9,
        textColor=rl_colors.HexColor("#64748B"),
    ))
    styles.add(ParagraphStyle(
        "TamgaMono", parent=styles["Normal"],
        fontName="Courier", fontSize=8, leading=10,
        textColor=rl_colors.HexColor("#020617"),
    ))
    return styles


def _coverage_color(coverage: str):
    """Return a ReportLab color for coverage status."""
    if coverage == "full":
        return _TAMGA_GREEN
    elif coverage == "partial":
        return _TAMGA_AMBER
    return _TAMGA_RED


def _coverage_badge(coverage: str) -> str:
    """Return a label for coverage status."""
    return {"full": "COMPLIANT", "partial": "PARTIAL", "missing": "GAP"}.get(coverage, "UNKNOWN")


def generate_owasp_pdf_report() -> bytes:
    """Generate OWASP LLM Top 10 compliance report as PDF bytes.

    Raises ImportError when ReportLab is unavailable.
    """
    if not _REPORTLAB_AVAILABLE:
        raise ImportError("ReportLab not installed")

    buf = io.BytesIO()
    styles = _build_pdf_styles()
    doc = SimpleDocTemplate(
        buf, pagesize=A4,
        leftMargin=2 * cm, rightMargin=2 * cm,
        topMargin=2 * cm, bottomMargin=2 * cm,
        title="OWASP LLM Top 10 Compliance Report — Tamga",
        author="Tamga Analyzer",
    )

    story: list = []

    # Title
    story.append(Paragraph("OWASP LLM Top 10 (2025)<br/>Compliance Report", styles["TamgaTitle"]))
    story.append(Paragraph(
        f"Generated: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')}  |  "
        f"Analyzer: Tamga v0.2.0",
        styles["TamgaSmall"],
    ))
    story.append(Spacer(1, 8 * mm))

    # Summary
    report = compute_compliance_report()
    story.append(Paragraph(f"Overall OWASP Coverage: {report.owasp_coverage_pct}%", styles["TamgaH2"]))
    story.append(Paragraph(
        f"Full: {sum(1 for r in OWASP_LLM_TOP_10_2025 if r.tamga_coverage == 'full')}  |  "
        f"Partial: {sum(1 for r in OWASP_LLM_TOP_10_2025 if r.tamga_coverage == 'partial')}  |  "
        f"Missing: {sum(1 for r in OWASP_LLM_TOP_10_2025 if r.tamga_coverage == 'missing')}  |  "
        f"Overall Score: {report.overall_score}/100",
        styles["TamgaBody"],
    ))
    story.append(Spacer(1, 5 * mm))

    # Risk table
    table_data = [["Risk", "Title", "Coverage", "Scanners"]]
    for risk in OWASP_LLM_TOP_10_2025:
        table_data.append([
            risk.id,
            risk.title,
            _coverage_badge(risk.tamga_coverage),
            ", ".join(risk.mitigating_scanners) if risk.mitigating_scanners else "—",
        ])

    col_widths = [14 * mm, 62 * mm, 24 * mm, 70 * mm]
    t = Table(table_data, colWidths=col_widths, repeatRows=1)
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), _TAMGA_NAVY),
        ("TEXTCOLOR", (0, 0), (-1, 0), rl_colors.white),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("FONTSIZE", (0, 0), (-1, 0), 8),
        ("FONTSIZE", (0, 1), (-1, -1), 7),
        ("ALIGN", (0, 0), (0, -1), "CENTER"),
        ("ALIGN", (2, 0), (2, -1), "CENTER"),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("GRID", (0, 0), (-1, -1), 0.5, rl_colors.HexColor("#E2E8F0")),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [rl_colors.white, _TAMGA_BG]),
        ("TOPPADDING", (0, 0), (-1, -1), 3),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 3),
    ]))
    story.append(t)
    story.append(Spacer(1, 8 * mm))

    # Per-risk details
    story.append(Paragraph("Detailed Risk Assessment", styles["TamgaH2"]))
    for risk in OWASP_LLM_TOP_10_2025:
        story.append(Paragraph(
            f"<b>{risk.id}: {risk.title}</b> — "
            f"<font color=\"{_coverage_color(risk.tamga_coverage)}\">{_coverage_badge(risk.tamga_coverage)}</font>",
            styles["TamgaBody"],
        ))
        story.append(Paragraph(risk.description, styles["TamgaSmall"]))
        if risk.recommendations:
            for rec in risk.recommendations:
                story.append(Paragraph(f"  • {rec}", styles["TamgaSmall"]))
        story.append(Spacer(1, 2 * mm))

    # Privacy section
    story.append(Spacer(1, 5 * mm))
    story.append(Paragraph("Privacy Entity Coverage", styles["TamgaH2"]))
    priv_data = [["Entity", "Regulation", "Supported"]]
    for e in PRIVACY_ENTITIES:
        priv_data.append([
            e.entity,
            e.regulation,
            "Yes" if e.tamga_supported else "No",
        ])
    t2 = Table(priv_data, colWidths=[60 * mm, 40 * mm, 30 * mm], repeatRows=1)
    t2.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), _TAMGA_NAVY),
        ("TEXTCOLOR", (0, 0), (-1, 0), rl_colors.white),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("FONTSIZE", (0, 0), (-1, -1), 7),
        ("GRID", (0, 0), (-1, -1), 0.5, rl_colors.HexColor("#E2E8F0")),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [rl_colors.white, _TAMGA_BG]),
        ("TOPPADDING", (0, 0), (-1, -1), 2),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 2),
    ]))
    story.append(t2)

    # Footer
    story.append(Spacer(1, 10 * mm))
    story.append(Paragraph(
        "This report was generated by Tamga Analyzer. "
        "Coverage assessments are based on the currently deployed scanner configuration.",
        styles["TamgaSmall"],
    ))

    doc.build(story)
    return buf.getvalue()


def generate_incident_pdf_report(stats: dict[str, Any] | None = None) -> bytes:
    """Generate security incident summary report as PDF bytes.

    Raises ImportError when ReportLab is unavailable.
    """
    if not _REPORTLAB_AVAILABLE:
        raise ImportError("ReportLab not installed")

    if stats is None:
        stats = {}

    buf = io.BytesIO()
    styles = _build_pdf_styles()
    doc = SimpleDocTemplate(
        buf, pagesize=A4,
        leftMargin=2 * cm, rightMargin=2 * cm,
        topMargin=2 * cm, bottomMargin=2 * cm,
        title="Security Incident Report — Tamga",
        author="Tamga Analyzer",
    )

    story: list = []

    story.append(Paragraph("Security Incident<br/>Summary Report", styles["TamgaTitle"]))
    story.append(Paragraph(
        f"Generated: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')}  |  "
        f"Period: Last {stats.get('period_hours', 24)} hours",
        styles["TamgaSmall"],
    ))
    story.append(Spacer(1, 8 * mm))

    # KPI cards (styled as a table)
    kpi_data = [
        ["Total Requests", "Blocked", "Redacted", "Warned"],
        [
            str(stats.get("total_requests", 0)),
            str(stats.get("blocked", 0)),
            str(stats.get("redacted", 0)),
            str(stats.get("warned", 0)),
        ],
    ]
    t = Table(kpi_data, colWidths=[45 * mm, 40 * mm, 40 * mm, 40 * mm])
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), _TAMGA_NAVY),
        ("TEXTCOLOR", (0, 0), (-1, 0), rl_colors.white),
        ("BACKGROUND", (0, 1), (-1, 1), _TAMGA_BG),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("FONTNAME", (0, 1), (-1, 1), "Helvetica-Bold"),
        ("FONTSIZE", (0, 0), (-1, 0), 9),
        ("FONTSIZE", (0, 1), (-1, 1), 16),
        ("ALIGN", (0, 0), (-1, -1), "CENTER"),
        ("GRID", (0, 0), (-1, -1), 0.5, rl_colors.HexColor("#E2E8F0")),
        ("TOPPADDING", (0, 0), (-1, -1), 6),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 6),
    ]))
    story.append(t)
    story.append(Spacer(1, 6 * mm))

    story.append(Paragraph(
        f"Avg Risk Score: {stats.get('avg_risk_score', 0):.2f}  |  "
        f"P95 Scan Latency: {stats.get('p95_scan_latency_ms', 0):.1f} ms",
        styles["TamgaBody"],
    ))

    if stats.get("top_finding_types"):
        story.append(Spacer(1, 5 * mm))
        story.append(Paragraph("Top Finding Types", styles["TamgaH2"]))
        for ft in stats["top_finding_types"][:10]:
            story.append(Paragraph(
                f"  <b>{ft.get('type', '?')}</b>: {ft.get('count', 0)} occurrences",
                styles["TamgaBody"],
            ))

    story.append(Spacer(1, 10 * mm))
    story.append(Paragraph(
        "Confidential — Generated by Tamga Analyzer",
        styles["TamgaSmall"],
    ))

    doc.build(story)
    return buf.getvalue()


