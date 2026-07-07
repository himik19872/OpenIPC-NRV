import React from 'react';
import { View, Text, StyleSheet } from 'react-native';

export default function RecordingsScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.title}>Записи</Text>
      <View style={styles.empty}>
        <Text style={styles.emptyText}>Выберите камеру для просмотра записей</Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: '#f5f5f5', padding: 16 },
  title: { fontSize: 24, fontWeight: '700', marginBottom: 16 },
  empty: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  emptyText: { color: '#888', fontSize: 16 },
});