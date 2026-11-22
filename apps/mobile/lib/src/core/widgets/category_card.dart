import 'package:flutter/material.dart';

import '../brand/brand_mark_path.dart';
import '../theme/theme.dart';
import '../theme/tokens.dart';

/// A category card in the shape language the web uses: a shaped window with
/// a lighter halo band, the name bottom-left and a round arrow bottom-right.
///
/// Mobile categories carry no photo, so the window holds the brand gradient
/// and the I CONFESS mark. The shape is chosen by the category's position, as
/// on the web (web/lib/card-shapes.ts holds the same geometry).
class CategoryCard extends StatelessWidget {
  const CategoryCard({
    required this.name,
    required this.index,
    required this.onTap,
    this.subtitle,
    super.key,
  });

  final String name;
  final int index;
  final String? subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final dark = Theme.of(context).brightness == Brightness.dark;
    final surfaces = AppSurfaces.of(context);
    final background = dark ? IConfess.colorBrand800 : IConfess.colorBrand100;
    final ink = dark ? IConfess.colorNeutral0 : IConfess.colorBrand900;

    return Semantics(
      button: true,
      label: name,
      excludeSemantics: true,
      child: Material(
        color: background,
        borderRadius: BorderRadius.circular(IConfess.radiusXl),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            // Name and arrow sit at the bottom whatever height the grid or
            // rail gives the card.
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              AspectRatio(
                aspectRatio: _artW / _artH,
                child: CustomPaint(
                  painter: _ShapePainter(
                    shape: _shapes[index % _shapes.length],
                    halo: IConfess.colorNeutral0.withValues(alpha: dark ? 0.08 : 0.55),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(
                  IConfess.space4, IConfess.space2, IConfess.space3, IConfess.space4),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Text(
                            name,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: IConfess.subheading.copyWith(color: ink, height: 1.2),
                          ),
                          if (subtitle != null)
                            Text(
                              subtitle!,
                              style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(width: IConfess.space2),
                    Container(
                      width: 36,
                      height: 36,
                      decoration: BoxDecoration(
                        color: dark
                            ? IConfess.colorNeutral0.withValues(alpha: 0.12)
                            : IConfess.colorNeutral0,
                        shape: BoxShape.circle,
                      ),
                      child: Icon(Icons.north_east_rounded, size: 16, color: ink),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

const double _artW = 300;
const double _artH = 230;

/// One shape: primitives in the 300x230 art box and a centre for the mark.
class _Shape {
  const _Shape(this.circles, this.polygon, this.cornerRadius, this.centre);
  final List<(double, double, double)> circles; // (cx, cy, r)
  final List<Offset> polygon;
  final double cornerRadius;
  final Offset centre;

  List<Path> paths() => [
        for (final (cx, cy, r) in circles)
          Path()..addOval(Rect.fromCircle(center: Offset(cx, cy), radius: r)),
        if (polygon.isNotEmpty) _roundedPolygon(polygon, cornerRadius),
      ];
}

const _shapes = <_Shape>[
  _Shape([(112.0, 82.0, 108.0)], [], 0, Offset(112, 82)), // circle
  _Shape([(150.0, 30.0, 54.0), (94.0, 90.0, 54.0), (206.0, 90.0, 54.0), (150.0, 148.0, 50.0), (150.0, 90.0, 60.0)], [], 0,
      Offset(150, 90)), // quatrefoil
  _Shape([], [Offset(24, -30), Offset(236, -30), Offset(236, 128), Offset(130, 206), Offset(24, 128)],
      34, Offset(130, 84)), // shield
  _Shape([], [Offset(136, -34), Offset(262, 90), Offset(136, 214), Offset(10, 90)], 36,
      Offset(136, 90)), // diamond
];

/// Cut each corner at [r] along both edges and join with a quadratic curve.
Path _roundedPolygon(List<Offset> pts, double r) {
  final path = Path();
  for (var i = 0; i < pts.length; i++) {
    final prev = pts[(i - 1 + pts.length) % pts.length];
    final cur = pts[i];
    final next = pts[(i + 1) % pts.length];
    final a = cur + (prev - cur) / (prev - cur).distance * r;
    final b = cur + (next - cur) / (next - cur).distance * r;
    if (i == 0) {
      path.moveTo(a.dx, a.dy);
    } else {
      path.lineTo(a.dx, a.dy);
    }
    path.quadraticBezierTo(cur.dx, cur.dy, b.dx, b.dy);
  }
  return path..close();
}

class _ShapePainter extends CustomPainter {
  _ShapePainter({required this.shape, required this.halo});

  final _Shape shape;
  final Color halo;

  @override
  void paint(Canvas canvas, Size size) {
    final scale = size.width / _artW;
    canvas.save();
    canvas.clipRect(Offset.zero & size);
    canvas.scale(scale);

    final paths = shape.paths();
    final haloPaint = Paint()
      ..color = halo
      ..style = PaintingStyle.stroke
      ..strokeWidth = 46
      ..strokeJoin = StrokeJoin.round;
    for (final p in paths) {
      canvas.drawPath(p, haloPaint);
    }

    final fill = Paint()
      ..shader = const LinearGradient(
        begin: Alignment.topLeft,
        end: Alignment.bottomRight,
        colors: [IConfess.colorBrand400, IConfess.colorBrand800],
      ).createShader(const Rect.fromLTWH(0, 0, _artW, _artH));
    for (final p in paths) {
      canvas.drawPath(p, fill);
    }

    // The mark, 56 units wide, centred in the shape.
    const markW = 56.0;
    final markH = markW * brandMarkHeight / brandMarkWidth;
    canvas.save();
    canvas.translate(shape.centre.dx - markW / 2, shape.centre.dy - markH / 2);
    canvas.scale(markW / brandMarkWidth);
    canvas.drawPath(
      brandMarkPath(),
      Paint()..color = IConfess.colorNeutral0.withValues(alpha: 0.9),
    );
    canvas.restore();

    canvas.restore();
  }

  @override
  bool shouldRepaint(_ShapePainter old) => old.shape != shape || old.halo != halo;
}

/// Width of a card in a horizontal rail.
const double categoryCardRailWidth = 168;

/// Height a rail needs for a card of [categoryCardRailWidth]: the art box
/// (168 x 230/300 = 129) plus a two-line name, its subtitle and padding.
const double categoryCardRailHeight = 232;
