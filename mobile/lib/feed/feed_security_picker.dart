import 'package:flutter/material.dart';

import '../api/securities_api.dart';
import '../search/asset_search_screen.dart';

class FeedSecurityPicker extends StatelessWidget {
  const FeedSecurityPicker({
    super.key,
    required this.selected,
    required this.onChanged,
    required this.maximum,
    this.enabled = true,
  });

  final List<Security> selected;
  final ValueChanged<List<Security>> onChanged;
  final int maximum;
  final bool enabled;

  Future<void> _pick(BuildContext context) async {
    final security = await Navigator.of(context).push<Security>(
      MaterialPageRoute(builder: (_) => const AssetSearchScreen(pick: true)),
    );
    if (!context.mounted || security == null) return;
    if (maximum == 1) {
      onChanged([security]);
    } else if (!selected.any((item) => item.secid == security.secid)) {
      onChanged([...selected, security]);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 4,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        for (final security in selected)
          InputChip(
            label: Text(security.secid),
            onDeleted: enabled
                ? () => onChanged(
                    selected
                        .where((item) => item.secid != security.secid)
                        .toList(),
                  )
                : null,
          ),
        if (maximum == 1 || selected.length < maximum)
          OutlinedButton.icon(
            icon: const Icon(Icons.add),
            onPressed: enabled ? () => _pick(context) : null,
            label: Text(
              selected.isNotEmpty && maximum == 1
                  ? 'Заменить бумагу'
                  : 'Выбрать бумагу',
            ),
          ),
      ],
    );
  }
}
