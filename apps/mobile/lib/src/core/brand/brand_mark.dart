import 'package:flutter/widgets.dart';

import 'brand_mark_path.dart';

/// The I CONFESS mark, painted from the path traced from the approved logo
/// (design/brand/mark.svg; regenerate with design/brand/generate.py).
///
/// [size] is the width; the height follows the mark's aspect ratio.
class BrandMarkGlyph extends StatelessWidget {
  const BrandMarkGlyph({required this.color, this.size = 40, super.key});

  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size * brandMarkHeight / brandMarkWidth,
      child: CustomPaint(painter: _BrandMarkPainter(color)),
    );
  }
}

class _BrandMarkPainter extends CustomPainter {
  _BrandMarkPainter(this.color);

  final Color color;

  // The path is built once per painter; shouldRepaint keeps it from being
  // rebuilt unless the colour changes.
  static final Path _mark = brandMarkPath();

  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / brandMarkWidth, size.height / brandMarkHeight);
    canvas.drawPath(_mark, Paint()..color = color..isAntiAlias = true);
    canvas.restore();
  }

  @override
  bool shouldRepaint(_BrandMarkPainter old) => old.color != color;
}
